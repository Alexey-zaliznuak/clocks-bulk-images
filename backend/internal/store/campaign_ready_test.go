package store

import "testing"

func TestCampaignReadyForVKUpload(t *testing.T) {
	if !CampaignReadyForVKUpload(CampaignRunning, 10, 8, 2, "") {
		t.Fatal("all terminal with successes should be ready")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 10, 0, 10, "") {
		t.Fatal("all failed should not upload")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 10, 4, 2, "") {
		t.Fatal("in-progress should not upload")
	}
	if CampaignReadyForVKUpload(CampaignDraft, 2, 2, 0, "") {
		t.Fatal("draft should not upload")
	}
	if CampaignReadyForVKUpload(CampaignRunning, 2, 2, 0, "123") {
		t.Fatal("already has a plan")
	}
}