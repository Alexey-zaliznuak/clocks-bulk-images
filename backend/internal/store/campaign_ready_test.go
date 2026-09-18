package store

import "testing"

func TestCampaignReadyForVKUpload(t *testing.T) {
	if !CampaignReadyForVKUpload(CampaignRunning, 10, 10, 0, false) {
		t.Fatal("all successful should upload automatically")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 10, 8, 2, false) {
		t.Fatal("failures without ignore must wait")
	}
	if !CampaignReadyForVKUpload(CampaignRunning, 10, 8, 2, true) {
		t.Fatal("ignored failures should upload")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 10, 0, 10, true) {
		t.Fatal("all failed should not upload")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 10, 4, 2, true) {
		t.Fatal("in-progress should not upload")
	}
	if CampaignReadyForVKUpload(CampaignDraft, 2, 2, 0, false) {
		t.Fatal("draft should not upload")
	}
	if !CampaignReadyForVKUpload(CampaignVKPlan, 2, 2, 0, false) {
		t.Fatal("vk_plan should stay eligible for upload")
	}
}

func TestCampaignUploadLocked(t *testing.T) {
	if campaignUploadLocked(CampaignRunning, "", false) {
		t.Fatal("running generation should still allow retries")
	}
	if !campaignUploadLocked(CampaignUploading, "", false) {
		t.Fatal("uploading must lock retries")
	}
	if !campaignUploadLocked(CampaignVKPlan, "", false) {
		t.Fatal("vk_plan must lock retries")
	}
	if !campaignUploadLocked(CampaignVKGroups, "", false) {
		t.Fatal("vk_groups must lock retries")
	}
	if !campaignUploadLocked(CampaignVKAds, "", false) {
		t.Fatal("vk_ads must lock retries")
	}
	if !campaignUploadLocked(CampaignRunning, "", true) {
		t.Fatal("ignored failures must lock retries")
	}
	if !campaignUploadLocked(CampaignRunning, "123", false) {
		t.Fatal("existing plan must lock retries")
	}
}