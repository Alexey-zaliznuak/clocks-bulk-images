package worker

import (
	"context"
	"fmt"
	"log"
	"strconv"

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
		if campaign.VKAdPlanID == "" {
			campaign.Lifecycle = store.CampaignRunning
		} else {
			campaign.Lifecycle = store.CampaignUploading
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

func (w *Worker) doVKUpload(ctx context.Context, campaign *store.AdCampaign) error {
	_ = w.store.TouchAdCampaign(ctx, campaign.ID)
	settings := campaign.VKSettings.Normalize()
	cat, err := w.vkads.ResolveCatalog(ctx, settings, campaign.CreatedAt)
	if err != nil {
		return err
	}
	if campaign.VKAdPlanID == "" {
		planID, err := w.vkads.CreateAdPlan(ctx, vkads.PlanBody(campaign.Title, settings, cat))
		if err != nil {
			return fmt.Errorf("создать кампанию ВКР: %w", err)
		}
		campaign.VKAdPlanID = vkads.FormatID(planID)
		if err := w.store.SaveAdCampaignVK(ctx, campaign); err != nil {
			return err
		}
	}
	planID, err := strconv.ParseInt(campaign.VKAdPlanID, 10, 64)
	if err != nil {
		return fmt.Errorf("id кампании ВКР: %w", err)
	}
	items, err := w.store.ListUploadableAdCampaignItems(ctx, campaign.ID)
	if err != nil {
		return err
	}
	var last error
	for _, item := range items {
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
		}
	}
	return last
}

