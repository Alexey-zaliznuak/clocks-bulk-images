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
		campaign.Lifecycle = store.CampaignUploading
		_ = w.store.SaveAdCampaignVK(ctx, campaign)
		return
	}
	campaign.VKUploadError = ""
	campaign.Lifecycle = store.CampaignCompleted
	if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
		log.Printf("worker: save vk campaign %s: %v", campaign.ID, err)
	}
}

func (w *Worker) doVKUpload(ctx context.Context, campaign *store.AdCampaign) error {
	_ = w.store.TouchAdCampaign(ctx, campaign.ID)
	settings := campaign.VKSettings.Normalize()
	log.Printf("worker: vk upload %s: resolve catalog", campaign.ID)
	cat, err := w.vkads.ResolveCatalog(ctx, settings, campaign.CreatedAt)
	if err != nil {
		return err
	}
	items, err := w.store.ListUploadableAdCampaignItems(ctx, campaign.ID)
	if err != nil {
		return err
	}
	patterns, err := w.vkads.ListBannerPatterns(ctx)
	if err != nil {
		log.Printf("worker: vk upload %s: banner_patterns: %v", campaign.ID, err)
	}

	banners := make([]map[string]any, len(items))
	for i, item := range items {
		if item.VKBannerID != "" {
			continue
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		log.Printf("worker: vk upload %s: upload video %s", campaign.ID, item.Value)
		banner, err := w.prepareBanner(ctx, item, settings, cat, patterns)
		if err != nil {
			return fmt.Errorf("видео %q: %w", item.Value, err)
		}
		banners[i] = banner
	}

	if campaign.VKAdPlanID == "" {
		if len(items) == 0 {
			return fmt.Errorf("нет групп для создания кампании ВКР")
		}
		groups := make([]map[string]any, 0, len(items))
		for i, item := range items {
			group := vkads.NestedGroupBody(item.Value, item.AudienceID, settings, cat)
			if banners[i] != nil {
				vkads.AttachBanner(group, banners[i])
			}
			groups = append(groups, group)
		}
		log.Printf("worker: vk upload %s: create ad_plan with %d groups", campaign.ID, len(groups))
		planID, created, err := w.vkads.CreateAdPlan(ctx, vkads.AttachCampaigns(vkads.PlanBody(campaign.Title, settings, cat), groups))
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
	var last error
	createdBanners := 0
	for i, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.VKBannerID != "" {
			continue
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		if banners[i] == nil {
			continue
		}
		oldGroup, _ := strconv.ParseInt(item.VKAdGroupID, 10, 64)
		group := vkads.GroupBody(item.Value, planID, item.AudienceID, settings, cat)
		vkads.AttachBanner(group, banners[i])
		log.Printf("worker: vk upload %s: create group+banner %s", campaign.ID, item.Value)
		got, err := w.vkads.CreateAdGroup(ctx, group)
		if err != nil {
			last = fmt.Errorf("группа/объявление %q: %w", item.Value, err)
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

func (w *Worker) prepareBanner(ctx context.Context, item *store.AdCampaignItem, settings vkads.Settings, cat *vkads.Catalog, patterns []vkads.BannerPattern) (map[string]any, error) {
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
	contentID, err := w.vkads.UploadVideo(ctx, item.Value+".mp4", f, info.Width, info.Height)
	if err != nil {
		return nil, fmt.Errorf("загрузить видео: %w", err)
	}
	text := item.NameTextTemplate
	if item.Kind == "surname" {
		text = item.SurnameTextTemplate
	}
	role := vkads.VideoRole(info.Width, info.Height)
	return vkads.BannerBody(
		item.Value,
		0,
		cat.URLID,
		contentID,
		settings.BannerTitle,
		adcampaign.RenderNameText(text, item.Value),
		vkads.CommunityCTA(settings.TargetAction),
		role,
		vkads.PickVideoBannerPattern(patterns, role),
	), nil
}
