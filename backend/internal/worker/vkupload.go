package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"named_clocks/backend/internal/adcampaign"
	"named_clocks/backend/internal/store"
	"named_clocks/backend/internal/vkads"
)

func (w *Worker) uploadCampaignToVK(ctx context.Context, campaign *store.AdCampaign) {
	if w.vkads == nil {
		campaign.VKUploadError = "VK Ads не настроен: задайте ZALEY_SECRET и ZALEY_ACCOUNT_NAME"
		campaign.Lifecycle = store.CampaignRunning
		_ = w.store.SaveAdCampaignVK(ctx, campaign)
		return
	}
	if err := w.doVKUpload(ctx, campaign); err != nil {
		log.Printf("worker: vk upload %s: %v", campaign.ID, err)
		campaign.VKUploadError = err.Error()
		if !store.CampaignVKUpload(campaign.Lifecycle) {
			campaign.Lifecycle = store.CampaignVKPlan
		}
		_ = w.store.SaveAdCampaignVK(ctx, campaign)
		return
	}
	campaign.VKUploadError = ""
	campaign.Lifecycle = store.CampaignCompleted
	if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
		log.Printf("worker: save vk campaign %s: %v", campaign.ID, err)
	}
}

func (w *Worker) setVKStage(ctx context.Context, campaign *store.AdCampaign, stage string) error {
	campaign.Lifecycle = stage
	campaign.VKUploadError = ""
	if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
		return err
	}
	return w.store.TouchAdCampaign(ctx, campaign.ID)
}

func (w *Worker) doVKUpload(ctx context.Context, campaign *store.AdCampaign) error {
	_ = w.store.TouchAdCampaign(ctx, campaign.ID)
	settings := campaign.VKSettings.Normalize()
	if err := w.setVKStage(ctx, campaign, store.CampaignVKPlan); err != nil {
		return err
	}
	log.Printf("worker: vk upload %s: resolve catalog", campaign.ID)
	cat, err := w.vkads.ResolveCatalog(ctx, settings, campaign.CreatedAt)
	if err != nil {
		return err
	}
	items, err := w.store.ListUploadableAdCampaignItems(ctx, campaign.ID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("нет групп для создания кампании ВКР")
	}

	if campaign.VKAdPlanID == "" {
		if err := w.setVKStage(ctx, campaign, store.CampaignVKPlan); err != nil {
			return err
		}
		log.Printf("worker: vk upload %s: create ad_plan", campaign.ID)
		planID, created, err := w.createAdPlan(ctx, campaign, settings, cat, items)
		if err != nil {
			return fmt.Errorf("создать кампанию ВКР: %w", err)
		}
		campaign.VKAdPlanID = vkads.FormatID(planID)
		if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
			return err
		}
		if err := w.saveCreatedGroups(ctx, items, created); err != nil {
			return err
		}
		log.Printf("worker: vk upload %s: ad_plan %s, groups %d", campaign.ID, campaign.VKAdPlanID, len(created))
	}

	planID, err := strconv.ParseInt(campaign.VKAdPlanID, 10, 64)
	if err != nil {
		return fmt.Errorf("id кампании ВКР: %w", err)
	}

	if err := w.setVKStage(ctx, campaign, store.CampaignVKGroups); err != nil {
		return err
	}
	var last error
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.VKAdGroupID != "" {
			continue
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		log.Printf("worker: vk upload %s: create group %s", campaign.ID, item.Value)
		got, err := w.vkads.CreateAdGroup(ctx, vkads.GroupBody(item.Value, planID, item.AudienceID, settings, cat))
		if err != nil {
			last = fmt.Errorf("группа %q: %w", item.Value, err)
			log.Printf("worker: vk group %s/%s: %v", campaign.ID, item.Value, err)
			continue
		}
		if err := w.persistCreated(ctx, item, got); err != nil {
			last = err
		}
	}
	if last != nil {
		return last
	}

	if err := w.setVKStage(ctx, campaign, store.CampaignVKAds); err != nil {
		return err
	}
	createdBanners := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.VKBannerID != "" {
			continue
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		log.Printf("worker: vk upload %s: upload video %s", campaign.ID, item.Value)
		banner, err := w.prepareBanner(ctx, item, settings, cat)
		if err != nil {
			last = fmt.Errorf("видео %q: %w", item.Value, err)
			log.Printf("worker: vk banner %s/%s: %v", campaign.ID, item.Value, err)
			continue
		}
		oldGroup, _ := strconv.ParseInt(item.VKAdGroupID, 10, 64)
		group := vkads.GroupBody(item.Value, planID, item.AudienceID, settings, cat)
		vkads.AttachBanner(group, banner)
		log.Printf("worker: vk upload %s: create group+banner %s", campaign.ID, item.Value)
		got, err := w.vkads.CreateAdGroup(ctx, group)
		if err != nil {
			last = fmt.Errorf("объявление %q: %w", item.Value, err)
			log.Printf("worker: vk group %s/%s: %v", campaign.ID, item.Value, err)
			continue
		}
		if err := w.persistCreated(ctx, item, got); err != nil {
			last = err
			continue
		}
		if oldGroup > 0 && oldGroup != got.ID {
			if delErr := w.vkads.DeleteAdGroup(ctx, oldGroup); delErr != nil {
				log.Printf("worker: vk upload %s: delete empty group %d: %v", campaign.ID, oldGroup, delErr)
			}
		}
		createdBanners += len(got.BannerIDs)
		log.Printf("worker: vk upload %s: group %s = %d banner=%v", campaign.ID, item.Value, got.ID, got.BannerIDs)
	}
	if last != nil {
		return last
	}
	log.Printf("worker: vk upload %s: done plan=%s banners=%d", campaign.ID, campaign.VKAdPlanID, createdBanners)
	return nil
}

func (w *Worker) createAdPlan(ctx context.Context, campaign *store.AdCampaign, settings vkads.Settings, cat *vkads.Catalog, items []*store.AdCampaignItem) (int64, []vkads.CreatedGroup, error) {
	groups := make([]map[string]any, 0, len(items))
	for _, item := range items {
		groups = append(groups, vkads.NestedGroupBody(item.Value, item.AudienceID, settings, cat))
	}
	log.Printf("worker: vk upload %s: ad_plan with %d groups", campaign.ID, len(groups))
	return w.vkads.CreateAdPlanWithGroups(ctx, vkads.PlanBody(campaign.Title, settings, cat), groups)
}

func (w *Worker) saveCreatedGroups(ctx context.Context, items []*store.AdCampaignItem, created []vkads.CreatedGroup) error {
	for i, got := range created {
		if i >= len(items) {
			break
		}
		if err := w.persistCreated(ctx, items[i], got); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) persistCreated(ctx context.Context, item *store.AdCampaignItem, got vkads.CreatedGroup) error {
	if got.ID != 0 {
		if err := w.store.SetAdCampaignItemGroupID(ctx, item.ID, vkads.FormatID(got.ID)); err != nil {
			return err
		}
		item.VKAdGroupID = vkads.FormatID(got.ID)
	}
	bannerID := firstID(got.BannerIDs)
	if bannerID == 0 && got.ID != 0 {
		ids, err := w.vkads.ListGroupBannerIDs(ctx, got.ID)
		if err != nil {
			log.Printf("worker: vk banners for group %d: %v", got.ID, err)
		}
		bannerID = firstID(ids)
	}
	if bannerID != 0 {
		if err := w.store.SetAdCampaignItemBannerID(ctx, item.ID, vkads.FormatID(bannerID)); err != nil {
			return err
		}
		item.VKBannerID = vkads.FormatID(bannerID)
	}
	return nil
}

func firstID(ids []int64) int64 {
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func (w *Worker) prepareBanner(ctx context.Context, item *store.AdCampaignItem, settings vkads.Settings, cat *vkads.Catalog) (map[string]any, error) {
	video := item.VideoObject
	if video == "" {
		video = item.SourceVideoObject
	}
	if video == "" {
		return nil, fmt.Errorf("нет видео")
	}
	if w.ffmpeg == nil || w.storage == nil {
		return nil, fmt.Errorf("нет ffmpeg/storage для загрузки объявления")
	}
	dir, err := os.MkdirTemp(w.ffmpeg.TempDir(), "vk-banner-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, "ad.mp4")
	if err := w.fetchToFile(ctx, video, local); err != nil {
		return nil, fmt.Errorf("скачать видео: %w", err)
	}
	info, err := w.ffmpeg.Probe(ctx, local)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(local)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	videoID, err := w.vkads.UploadVideo(ctx, item.Value+".mp4", f, info.Width, info.Height)
	if err != nil {
		return nil, fmt.Errorf("загрузить видео: %w", err)
	}
	role := vkads.VideoRole(info.Width, info.Height)
	pattern := vkads.PickPackageBannerPattern(cat.Patterns, role, item.ImageObject != "")
	if pattern == nil {
		return nil, fmt.Errorf("в пакете нет паттерна объявления")
	}
	log.Printf("worker: vk upload banner %s: pattern %d %s [%s]",
		item.Value, pattern.ID, pattern.Name, vkads.PatternSlotSummary(pattern))
	images := map[string]int64{}
	if item.ImageObject != "" {
		for _, role := range vkads.RequiredImageRoles(pattern) {
			id, err := w.uploadBannerImage(ctx, item, role)
			if err != nil {
				return nil, fmt.Errorf("загрузить картинку %s: %w", role, err)
			}
			images[role] = id
		}
	}
	text := item.NameTextTemplate
	if item.Kind == "surname" {
		text = item.SurnameTextTemplate
	}
	return vkads.BannerBody(
		item.Value,
		0,
		cat.URLID,
		videoID,
		images,
		settings.BannerTitle,
		adcampaign.RenderNameText(text, item.Value),
		vkads.CommunityCTA(settings.TargetAction),
		pattern,
	), nil
}

func (w *Worker) uploadBannerImage(ctx context.Context, item *store.AdCampaignItem, role string) (int64, error) {
	dir, err := os.MkdirTemp(w.ffmpeg.TempDir(), "vk-image-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, "ad.jpg")
	if err := w.fetchToFile(ctx, item.ImageObject, local); err != nil {
		return 0, err
	}
	width, height := vkads.RoleImageSize(role)
	sized := filepath.Join(dir, "sized.jpg")
	if err := w.ffmpeg.ResizeImage(ctx, local, sized, width, height); err != nil {
		return 0, err
	}
	f, err := os.Open(sized)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return w.vkads.UploadStatic(ctx, fmt.Sprintf("%s_%s.jpg", item.Value, role), f, width, height)
}
