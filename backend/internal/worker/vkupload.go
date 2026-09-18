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
	var pendingGroups []*store.AdCampaignItem
	for _, item := range items {
		if item.VKAdGroupID == "" {
			pendingGroups = append(pendingGroups, item)
		}
	}
	if campaign.VKAdPlanID == "" {
		if len(pendingGroups) == 0 {
			return fmt.Errorf("нет групп для создания кампании ВКР")
		}
		groups := make([]map[string]any, 0, len(pendingGroups))
		for _, item := range pendingGroups {
			groups = append(groups, vkads.NestedGroupBody(item.Value, item.AudienceID, settings, cat))
		}
		log.Printf("worker: vk upload %s: create ad_plan with %d groups", campaign.ID, len(groups))
		planID, groupIDs, err := w.vkads.CreateAdPlan(ctx, vkads.AttachCampaigns(vkads.PlanBody(campaign.Title, settings, cat), groups))
		if err != nil {
			return fmt.Errorf("создать кампанию ВКР: %w", err)
		}
		campaign.VKAdPlanID = vkads.FormatID(planID)
		if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
			return err
		}
		for i, id := range groupIDs {
			if i >= len(pendingGroups) {
				break
			}
			if err := w.store.SetAdCampaignItemGroupID(ctx, pendingGroups[i].ID, vkads.FormatID(id)); err != nil {
				return err
			}
			pendingGroups[i].VKAdGroupID = vkads.FormatID(id)
		}
		log.Printf("worker: vk upload %s: ad_plan %s, groups %d", campaign.ID, campaign.VKAdPlanID, len(groupIDs))
	}
	planID, err := strconv.ParseInt(campaign.VKAdPlanID, 10, 64)
	if err != nil {
		return fmt.Errorf("id кампании ВКР: %w", err)
	}
	var last error
	for _, item := range pendingGroups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		if item.VKAdGroupID != "" {
			continue
		}
		id, err := w.vkads.CreateAdGroup(ctx, vkads.GroupBody(item.Value, planID, item.AudienceID, settings, cat))
		if err != nil {
			last = fmt.Errorf("группа %q: %w", item.Value, err)
			log.Printf("worker: vk group %s/%s: %v", campaign.ID, item.Value, err)
			continue
		}
		if err := w.store.SetAdCampaignItemGroupID(ctx, item.ID, vkads.FormatID(id)); err != nil {
			last = err
		} else {
			item.VKAdGroupID = vkads.FormatID(id)
			log.Printf("worker: vk upload %s: group %s = %d", campaign.ID, item.Value, id)
		}
	}
	if last != nil {
		return last
	}

	patterns, err := w.vkads.ListBannerPatterns(ctx)
	if err != nil {
		log.Printf("worker: vk upload %s: banner_patterns: %v", campaign.ID, err)
	}
	created := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.VKAdGroupID == "" || item.VKBannerID != "" {
			continue
		}
		_ = w.store.TouchAdCampaign(ctx, campaign.ID)
		log.Printf("worker: vk upload %s: banner %s", campaign.ID, item.Value)
		id, err := w.createItemBanner(ctx, item, settings, cat, patterns)
		if err != nil {
			last = fmt.Errorf("объявление %q: %w", item.Value, err)
			log.Printf("worker: vk banner %s/%s: %v", campaign.ID, item.Value, err)
			continue
		}
		if err := w.store.SetAdCampaignItemBannerID(ctx, item.ID, vkads.FormatID(id)); err != nil {
			last = err
			continue
		}
		item.VKBannerID = vkads.FormatID(id)
		created++
		log.Printf("worker: vk upload %s: banner %s = %d", campaign.ID, item.Value, id)
	}
	if last != nil {
		return last
	}
	log.Printf("worker: vk upload %s: done plan=%s banners=%d", campaign.ID, campaign.VKAdPlanID, created)
	return nil
}

func (w *Worker) createItemBanner(ctx context.Context, item *store.AdCampaignItem, settings vkads.Settings, cat *vkads.Catalog, patterns []vkads.BannerPattern) (int64, error) {
	groupID, err := strconv.ParseInt(item.VKAdGroupID, 10, 64)
	if err != nil || groupID == 0 {
		return 0, fmt.Errorf("id группы ВКР")
	}
	video := item.VideoObject
	if video == "" {
		video = item.SourceVideoObject
	}
	if video == "" {
		return 0, fmt.Errorf("нет видео")
	}
	if w.ffmpeg == nil || w.storage == nil {
		return 0, fmt.Errorf("нет ffmpeg/storage для загрузки объявления")
	}
	dir, err := os.MkdirTemp(w.ffmpeg.TempDir(), "vk-banner-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, "ad.mp4")
	if err := w.fetchToFile(ctx, video, local); err != nil {
		return 0, fmt.Errorf("скачать видео: %w", err)
	}
	info, err := w.ffmpeg.Probe(ctx, local)
	if err != nil {
		return 0, err
	}
	f, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	contentID, err := w.vkads.UploadVideo(ctx, item.Value+".mp4", f, info.Width, info.Height)
	if err != nil {
		return 0, fmt.Errorf("загрузить видео: %w", err)
	}
	role := vkads.VideoRole(info.Width, info.Height)
	pattern := vkads.PickVideoBannerPattern(patterns, role)
	text := item.NameTextTemplate
	if item.Kind == "surname" {
		text = item.SurnameTextTemplate
	}
	body := vkads.BannerBody(
		item.Value,
		groupID,
		cat.URLID,
		contentID,
		settings.BannerTitle,
		adcampaign.RenderNameText(text, item.Value),
		vkads.CommunityCTA(settings.TargetAction),
		role,
		pattern,
	)
	return w.vkads.CreateBanner(ctx, body)
}
