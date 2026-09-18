package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"named_clocks/backend/internal/vkads"
)

const (
	CampaignDraft     = "draft"
	CampaignRunning   = "running"
	CampaignUploading = "uploading"
	CampaignCompleted = "completed"
)

var (
	ErrCampaignNotDraft       = errors.New("campaign is not draft")
	ErrCampaignInProgress     = errors.New("есть задачи, которые ещё не завершились")
	ErrNothingToUpload        = errors.New("нет успешных задач для загрузки в ВКР")
	ErrCampaignNotRunning     = errors.New("кампания не в состоянии генерации")
	ErrCampaignUploadStarted  = errors.New("после перехода к загрузке в ВКР ошибки нельзя повторить")
)

type AdCampaign struct {
	ID                  string            `json:"id"`
	Title               string            `json:"title"`
	Lifecycle           string            `json:"lifecycle"`
	NameTextTemplate    string            `json:"nameTextTemplate"`
	SurnameTextTemplate string            `json:"surnameTextTemplate"`
	TemplateID          string            `json:"templateId"`
	ImageSettings       map[string]string `json:"imageSettings"`
	NameSettingKey      string            `json:"nameSettingKey"`
	VideoModel          string            `json:"videoModel"`
	VideoPrompt         string            `json:"videoPrompt"`
	VideoDuration       *int              `json:"videoDuration,omitempty"`
	VideoResolution     string            `json:"videoResolution"`
	VideoAspectRatio    string            `json:"videoAspectRatio"`
	GenerateAudio       bool              `json:"generateAudio"`
	AudioAssetID        string            `json:"audioAssetId,omitempty"`
	AudioObject         string            `json:"-"`
	CreatedAt           time.Time         `json:"createdAt"`
	StartedAt           *time.Time        `json:"startedAt,omitempty"`
	UpdatedAt           time.Time         `json:"updatedAt"`
	Total               int               `json:"total"`
	NameCount           int               `json:"nameCount"`
	SurnameCount        int               `json:"surnameCount"`
	Done                int               `json:"done"`
	Failed              int               `json:"failed"`
	CostUSD             float64           `json:"costUsd"`
	CostRUB             float64           `json:"costRub"`
	VKSettings          vkads.Settings    `json:"vkSettings"`
	VKAdPlanID          string            `json:"vkAdPlanId,omitempty"`
	VKUploadError       string            `json:"vkUploadError,omitempty"`
	VKIgnoreFailed      bool              `json:"vkIgnoreFailed"`
	Names               []string          `json:"names,omitempty"`
	Surnames            []string          `json:"surnames,omitempty"`
}

type AdCampaignItem struct {
	ID                  string            `json:"id"`
	CampaignID          string            `json:"campaignId"`
	Kind                string            `json:"kind"`
	Value               string            `json:"value"`
	Status              string            `json:"status"`
	Error               string            `json:"error"`
	Attempts            int               `json:"attempts"`
	ImanatorOrderID     string            `json:"imanatorOrderId"`
	ImageURL            string            `json:"imageUrl"`
	ImageObject         string            `json:"imageObject"`
	OpenRouterJobID     string            `json:"openrouterJobId"`
	SourceVideoObject   string            `json:"sourceVideoObject"`
	VideoObject         string            `json:"videoObject"`
	CostUSD             float64           `json:"costUsd"`
	CostRUB             float64           `json:"costRub"`
	ImageDownloadURL    string            `json:"imageDownloadUrl,omitempty"`
	SourceDownloadURL   string            `json:"sourceDownloadUrl,omitempty"`
	VideoDownloadURL    string            `json:"videoDownloadUrl,omitempty"`
	AudienceID          int64             `json:"audienceId,omitempty"`
	AudienceName        string            `json:"audienceName,omitempty"`
	VKAdGroupID         string            `json:"vkAdGroupId,omitempty"`
	VKBannerID          string            `json:"vkBannerId,omitempty"`
	TemplateID          string            `json:"-"`
	ImageSettings       map[string]string `json:"-"`
	NameSettingKey      string            `json:"-"`
	NameTextTemplate    string            `json:"-"`
	SurnameTextTemplate string            `json:"-"`
	VideoModel          string            `json:"-"`
	VideoPrompt         string            `json:"-"`
	VideoDuration       *int              `json:"-"`
	VideoResolution     string            `json:"-"`
	VideoAspectRatio    string            `json:"-"`
	GenerateAudio       bool              `json:"-"`
	AudioObject         string            `json:"-"`
}

func (s *Store) CreateAdCampaign(ctx context.Context, c *AdCampaign, names, surnames []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	settings, _ := json.Marshal(c.ImageSettings)
	c.VKSettings = c.VKSettings.Normalize()
	vkSettings, _ := json.Marshal(c.VKSettings)
	var audioID sql.NullString
	if c.AudioAssetID != "" {
		audioID = sql.NullString{String: c.AudioAssetID, Valid: true}
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO ad_campaigns
		(title,name_text_template,surname_text_template,template_id,image_settings,name_setting_key,
		 video_model,video_prompt,video_duration,video_resolution,video_aspect_ratio,generate_audio,audio_asset_id,audio_object,vk_settings)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id,created_at,updated_at`,
		c.Title, c.NameTextTemplate, c.SurnameTextTemplate, c.TemplateID, settings, c.NameSettingKey,
		c.VideoModel, c.VideoPrompt, c.VideoDuration, c.VideoResolution, c.VideoAspectRatio, c.GenerateAudio, audioID, c.AudioObject, vkSettings,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert ad campaign: %w", err)
	}
	position := 0
	for _, group := range []struct {
		kind   string
		values []string
	}{{"name", names}, {"surname", surnames}} {
		kind, values := group.kind, group.values
		for _, value := range values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO ad_campaign_items(campaign_id,kind,value,position) VALUES($1,$2,$3,$4)`, c.ID, kind, value, position); err != nil {
				return fmt.Errorf("insert ad campaign item: %w", err)
			}
			position++
		}
	}
	c.Lifecycle = CampaignDraft
	c.NameCount = len(names)
	c.SurnameCount = len(surnames)
	c.Total = c.NameCount + c.SurnameCount
	return tx.Commit()
}

const campaignAggregate = `SELECT c.id,c.title,c.lifecycle,c.name_text_template,c.surname_text_template,c.template_id,c.image_settings,
	c.name_setting_key,c.video_model,c.video_prompt,c.video_duration,c.video_resolution,c.video_aspect_ratio,
	c.generate_audio,c.audio_asset_id,c.created_at,c.started_at,c.updated_at,c.vk_settings,c.vk_ad_plan_id,c.vk_upload_error,c.vk_ignore_failed,
	COUNT(i.id),COUNT(i.id) FILTER(WHERE i.kind='name'),COUNT(i.id) FILTER(WHERE i.kind='surname'),
	COUNT(i.id) FILTER(WHERE i.status='done'),COUNT(i.id) FILTER(WHERE i.status='failed'),COALESCE(SUM(i.cost_usd),0)
	FROM ad_campaigns c LEFT JOIN ad_campaign_items i ON i.campaign_id=c.id`

func scanCampaign(row interface{ Scan(...any) error }) (*AdCampaign, error) {
	var c AdCampaign
	var raw, vkRaw []byte
	var audio sql.NullString
	if err := row.Scan(&c.ID, &c.Title, &c.Lifecycle, &c.NameTextTemplate, &c.SurnameTextTemplate, &c.TemplateID, &raw,
		&c.NameSettingKey, &c.VideoModel, &c.VideoPrompt, &c.VideoDuration, &c.VideoResolution, &c.VideoAspectRatio,
		&c.GenerateAudio, &audio, &c.CreatedAt, &c.StartedAt, &c.UpdatedAt, &vkRaw, &c.VKAdPlanID, &c.VKUploadError, &c.VKIgnoreFailed,
		&c.Total, &c.NameCount, &c.SurnameCount, &c.Done, &c.Failed, &c.CostUSD); err != nil {
		return nil, err
	}
	c.AudioAssetID = audio.String
	_ = json.Unmarshal(raw, &c.ImageSettings)
	_ = json.Unmarshal(vkRaw, &c.VKSettings)
	c.VKSettings = c.VKSettings.Normalize()
	return &c, nil
}

func (s *Store) ListAdCampaigns(ctx context.Context) ([]*AdCampaign, error) {
	rows, err := s.db.QueryContext(ctx, campaignAggregate+` GROUP BY c.id ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AdCampaign
	for rows.Next() {
		c, e := scanCampaign(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetAdCampaign(ctx context.Context, id string) (*AdCampaign, error) {
	_ = s.parkBlockedVKUploads(ctx, id)
	c, err := scanCampaign(s.db.QueryRowContext(ctx, campaignAggregate+` WHERE c.id=$1 GROUP BY c.id`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil || c == nil {
		return c, err
	}
	if err := s.attachCampaignLists(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) attachCampaignLists(ctx context.Context, c *AdCampaign) error {
	rows, err := s.db.QueryContext(ctx, `SELECT kind,value FROM ad_campaign_items WHERE campaign_id=$1 ORDER BY position,id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Names, c.Surnames = nil, nil
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return err
		}
		if kind == "surname" {
			c.Surnames = append(c.Surnames, value)
		} else {
			c.Names = append(c.Names, value)
		}
	}
	return rows.Err()
}

func campaignUploadLocked(lifecycle, planID string, ignoreFailed bool) bool {
	return lifecycle == CampaignUploading || lifecycle == CampaignCompleted || ignoreFailed || planID != ""
}

func (s *Store) StartAdCampaign(ctx context.Context, id string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	var lifecycle string
	if err = tx.QueryRowContext(ctx, `SELECT lifecycle FROM ad_campaigns WHERE id=$1 FOR UPDATE`, id).Scan(&lifecycle); err != nil {
		return 0, err
	}
	if lifecycle != CampaignDraft {
		return 0, ErrCampaignNotDraft
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ad_campaigns SET lifecycle='running',started_at=now(),updated_at=now() WHERE id=$1`, id); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE ad_campaign_items SET status='audience_searching',updated_at=now() WHERE campaign_id=$1 AND status='draft'`, id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), tx.Commit()
}

func (s *Store) ListAdCampaignItems(ctx context.Context, campaignID, kind, status string, limit, offset int) ([]*AdCampaignItem, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ad_campaign_items WHERE campaign_id=$1 AND ($2='' OR kind=$2) AND ($3='' OR status=$3)`, campaignID, kind, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,campaign_id,kind,value,status,error,attempts,imanator_order_id,image_url,image_object,
		openrouter_job_id,source_video_object,video_object,cost_usd,audience_id,audience_name,vk_ad_group_id,vk_banner_id FROM ad_campaign_items
		WHERE campaign_id=$1 AND ($2='' OR kind=$2) AND ($3='' OR status=$3) ORDER BY position,id LIMIT $4 OFFSET $5`,
		campaignID, kind, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*AdCampaignItem
	for rows.Next() {
		var i AdCampaignItem
		if err := rows.Scan(&i.ID, &i.CampaignID, &i.Kind, &i.Value, &i.Status, &i.Error, &i.Attempts, &i.ImanatorOrderID, &i.ImageURL, &i.ImageObject, &i.OpenRouterJobID, &i.SourceVideoObject, &i.VideoObject, &i.CostUSD, &i.AudienceID, &i.AudienceName, &i.VKAdGroupID, &i.VKBannerID); err != nil {
			return nil, 0, err
		}
		out = append(out, &i)
	}
	return out, total, rows.Err()
}

const campaignItemColumns = `i.id,i.campaign_id,i.kind,i.value,i.status,i.error,i.attempts,i.imanator_order_id,i.image_url,i.image_object,
	i.openrouter_job_id,i.source_video_object,i.video_object,i.cost_usd,i.audience_id,i.audience_name,i.vk_ad_group_id,c.template_id,c.image_settings,c.name_setting_key,
	c.name_text_template,c.surname_text_template,c.video_model,c.video_prompt,c.video_duration,c.video_resolution,c.video_aspect_ratio,c.generate_audio,c.audio_object`

func scanClaimedCampaignItem(row interface{ Scan(...any) error }) (*AdCampaignItem, error) {
	var i AdCampaignItem
	var raw []byte
	err := row.Scan(&i.ID, &i.CampaignID, &i.Kind, &i.Value, &i.Status, &i.Error, &i.Attempts, &i.ImanatorOrderID, &i.ImageURL, &i.ImageObject,
		&i.OpenRouterJobID, &i.SourceVideoObject, &i.VideoObject, &i.CostUSD, &i.AudienceID, &i.AudienceName, &i.VKAdGroupID, &i.TemplateID, &raw, &i.NameSettingKey,
		&i.NameTextTemplate, &i.SurnameTextTemplate, &i.VideoModel, &i.VideoPrompt, &i.VideoDuration, &i.VideoResolution, &i.VideoAspectRatio, &i.GenerateAudio, &i.AudioObject)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &i.ImageSettings)
	if i.ImageSettings == nil {
		i.ImageSettings = map[string]string{}
	}
	return &i, nil
}

func (s *Store) ClaimNextAdCampaignItem(ctx context.Context, lease time.Duration) (*AdCampaignItem, error) {
	row := s.db.QueryRowContext(ctx, `WITH picked AS (
		SELECT i.id FROM ad_campaign_items i JOIN ad_campaigns c ON c.id=i.campaign_id
		WHERE c.lifecycle='running' AND i.status NOT IN ('draft','done','failed')
		AND (i.locked_at IS NULL OR i.locked_at<now()-make_interval(secs=>$1))
		AND (i.next_attempt_at IS NULL OR i.next_attempt_at<=now())
		ORDER BY i.created_at,i.position FOR UPDATE OF i SKIP LOCKED LIMIT 1)
		UPDATE ad_campaign_items i SET locked_at=now(),updated_at=now() FROM ad_campaigns c
		WHERE i.id=(SELECT id FROM picked) AND c.id=i.campaign_id RETURNING `+campaignItemColumns, lease.Seconds())
	i, err := scanClaimedCampaignItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return i, err
}

func (s *Store) SaveAdCampaignItem(ctx context.Context, i *AdCampaignItem) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET status=$2,error=$3,attempts=$4,imanator_order_id=$5,image_url=$6,image_object=$7,
		openrouter_job_id=$8,source_video_object=$9,video_object=$10,cost_usd=$11,audience_id=$12,audience_name=$13,vk_ad_group_id=$14,locked_at=now(),updated_at=now() WHERE id=$1`,
		i.ID, i.Status, i.Error, i.Attempts, i.ImanatorOrderID, i.ImageURL, i.ImageObject, i.OpenRouterJobID, i.SourceVideoObject, i.VideoObject, i.CostUSD, i.AudienceID, i.AudienceName, i.VKAdGroupID)
	return err
}
func (s *Store) TouchAdCampaignItem(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET locked_at=now() WHERE id=$1`, id)
	return e
}
func (s *Store) ReleaseAdCampaignItem(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET locked_at=NULL,updated_at=now() WHERE id=$1`, id)
	return e
}
func (s *Store) RescheduleAdCampaignItem(ctx context.Context, i *AdCampaignItem, d time.Duration) error {
	_, e := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET error=$2,attempts=$3,imanator_order_id=$4,image_url=$5,image_object=$6,openrouter_job_id=$7,source_video_object=$8,cost_usd=$9,audience_id=$10,audience_name=$11,next_attempt_at=now()+make_interval(secs=>$12),locked_at=NULL,updated_at=now() WHERE id=$1`, i.ID, i.Error, i.Attempts, i.ImanatorOrderID, i.ImageURL, i.ImageObject, i.OpenRouterJobID, i.SourceVideoObject, i.CostUSD, i.AudienceID, i.AudienceName, d.Seconds())
	return e
}

const campaignRetrySQL = `CASE WHEN source_video_object<>'' THEN 'audio_mixing' WHEN openrouter_job_id<>'' THEN 'video_polling' WHEN image_object<>'' THEN 'image_ready' WHEN audience_id<>0 THEN 'queued' ELSE 'audience_searching' END`

func (s *Store) RetryAdCampaignItem(ctx context.Context, id string) (string, error) {
	var lifecycle, planID string
	var ignore bool
	err := s.db.QueryRowContext(ctx, `SELECT c.lifecycle,c.vk_ad_plan_id,c.vk_ignore_failed
		FROM ad_campaign_items i JOIN ad_campaigns c ON c.id=i.campaign_id WHERE i.id=$1`, id).Scan(&lifecycle, &planID, &ignore)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if campaignUploadLocked(lifecycle, planID, ignore) {
		return "", ErrCampaignUploadStarted
	}
	var st string
	e := s.db.QueryRowContext(ctx, `UPDATE ad_campaign_items SET status=`+campaignRetrySQL+`,error='',attempts=0,next_attempt_at=NULL,locked_at=NULL,updated_at=now() WHERE id=$1 AND status='failed' RETURNING status`, id).Scan(&st)
	if e == sql.ErrNoRows {
		return "", nil
	}
	return st, e
}
func (s *Store) RetryAdCampaign(ctx context.Context, id string) (int, error) {
	var lifecycle, planID string
	var ignore bool
	err := s.db.QueryRowContext(ctx, `SELECT lifecycle,vk_ad_plan_id,vk_ignore_failed FROM ad_campaigns WHERE id=$1`, id).Scan(&lifecycle, &planID, &ignore)
	if err != nil {
		return 0, err
	}
	if campaignUploadLocked(lifecycle, planID, ignore) {
		return 0, ErrCampaignUploadStarted
	}
	r, e := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET status=`+campaignRetrySQL+`,error='',attempts=0,next_attempt_at=NULL,locked_at=NULL,updated_at=now() WHERE campaign_id=$1 AND status='failed'`, id)
	if e != nil {
		return 0, e
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}
func (s *Store) DeleteAdCampaign(ctx context.Context, id string) (bool, error) {
	r, e := s.db.ExecContext(ctx, `DELETE FROM ad_campaigns WHERE id=$1`, id)
	if e != nil {
		return false, e
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

func CampaignReadyForVKUpload(lifecycle string, total, done, failed int, ignoreFailed bool) bool {
	if lifecycle != CampaignRunning && lifecycle != CampaignUploading {
		return false
	}
	if total <= 0 || done+failed != total || done == 0 {
		return false
	}
	if failed > 0 && !ignoreFailed {
		return false
	}
	return true
}

func (s *Store) parkBlockedVKUploads(ctx context.Context, id string) error {
	const sqlText = `UPDATE ad_campaigns c SET lifecycle='running', updated_at=now()
		WHERE c.lifecycle='uploading' AND NOT c.vk_ignore_failed
		  AND EXISTS (SELECT 1 FROM ad_campaign_items i WHERE i.campaign_id=c.id AND i.status='failed')
		  AND NOT EXISTS (SELECT 1 FROM ad_campaign_items i WHERE i.campaign_id=c.id AND i.status NOT IN ('done','failed'))`
	if id == "" {
		_, err := s.db.ExecContext(ctx, sqlText)
		return err
	}
	_, err := s.db.ExecContext(ctx, sqlText+` AND c.id=$1`, id)
	return err
}

func (s *Store) IgnoreAdCampaignFailures(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var lifecycle string
	if err = tx.QueryRowContext(ctx, `SELECT lifecycle FROM ad_campaigns WHERE id=$1 FOR UPDATE`, id).Scan(&lifecycle); err != nil {
		return err
	}
	if lifecycle != CampaignRunning && lifecycle != CampaignUploading {
		return ErrCampaignNotRunning
	}
	var total, done, failed int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status='done'), COUNT(*) FILTER (WHERE status='failed')
		FROM ad_campaign_items WHERE campaign_id=$1`, id).Scan(&total, &done, &failed); err != nil {
		return err
	}
	if total == 0 || done+failed != total {
		return ErrCampaignInProgress
	}
	if done == 0 {
		return ErrNothingToUpload
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ad_campaigns SET vk_ignore_failed=true, vk_upload_error='', lifecycle='running', updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClaimCampaignForVKUpload(ctx context.Context, stale time.Duration) (*AdCampaign, error) {
	if stale <= 0 {
		stale = 2 * time.Minute
	}
	_ = s.parkBlockedVKUploads(ctx, "")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var id string
	err = tx.QueryRowContext(ctx, `SELECT c.id FROM ad_campaigns c
		WHERE c.lifecycle IN ('running','uploading','completed')
		  AND (
		    c.vk_ad_plan_id=''
		    OR EXISTS (
		      SELECT 1 FROM ad_campaign_items i
		      WHERE i.campaign_id=c.id AND i.status='done' AND i.audience_id<>0 AND i.vk_ad_group_id=''
		    )
		    OR EXISTS (
		      SELECT 1 FROM ad_campaign_items i
		      WHERE i.campaign_id=c.id AND i.status='done' AND i.vk_ad_group_id<>'' AND i.vk_banner_id=''
		    )
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM ad_campaign_items i
		    WHERE i.campaign_id=c.id AND i.status NOT IN ('done','failed')
		  )
		  AND EXISTS (
		    SELECT 1 FROM ad_campaign_items i
		    WHERE i.campaign_id=c.id AND i.status='done' AND i.audience_id<>0
		  )
		  AND (
		    NOT EXISTS (SELECT 1 FROM ad_campaign_items i WHERE i.campaign_id=c.id AND i.status='failed')
		    OR c.vk_ignore_failed
		  )
		  AND (c.lifecycle='running' OR c.updated_at < now()-make_interval(secs=>$1))
		ORDER BY c.updated_at
		FOR UPDATE OF c SKIP LOCKED
		LIMIT 1`, stale.Seconds()).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ad_campaigns SET lifecycle='uploading',vk_upload_error='',updated_at=now() WHERE id=$1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetAdCampaign(ctx, id)
}

func (s *Store) TouchAdCampaign(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaigns SET updated_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) SaveAdCampaignVK(ctx context.Context, c *AdCampaign) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaigns SET lifecycle=$2,vk_ad_plan_id=$3,vk_upload_error=$4,updated_at=now() WHERE id=$1`,
		c.ID, c.Lifecycle, c.VKAdPlanID, c.VKUploadError)
	return err
}

func (s *Store) MarkCampaignCompletedIfNothingToUpload(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaigns SET lifecycle='completed',updated_at=now()
		WHERE id=$1 AND lifecycle='running'
		AND NOT EXISTS (SELECT 1 FROM ad_campaign_items WHERE campaign_id=$1 AND status NOT IN ('done','failed'))
		AND NOT EXISTS (SELECT 1 FROM ad_campaign_items WHERE campaign_id=$1 AND status='done' AND audience_id<>0)`, id)
	return err
}

func (s *Store) SetAdCampaignItemGroupID(ctx context.Context, id, groupID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET vk_ad_group_id=$2,updated_at=now() WHERE id=$1`, id, groupID)
	return err
}

func (s *Store) SetAdCampaignItemBannerID(ctx context.Context, id, bannerID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ad_campaign_items SET vk_banner_id=$2,updated_at=now() WHERE id=$1`, id, bannerID)
	return err
}

func (s *Store) ListUploadableAdCampaignItems(ctx context.Context, campaignID string) ([]*AdCampaignItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id,i.campaign_id,i.kind,i.value,i.status,i.audience_id,i.audience_name,
		i.vk_ad_group_id,i.vk_banner_id,i.video_object,i.source_video_object,c.name_text_template,c.surname_text_template
		FROM ad_campaign_items i JOIN ad_campaigns c ON c.id=i.campaign_id
		WHERE i.campaign_id=$1 AND i.status='done' AND i.audience_id<>0 ORDER BY i.position,i.id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AdCampaignItem
	for rows.Next() {
		var i AdCampaignItem
		if err := rows.Scan(&i.ID, &i.CampaignID, &i.Kind, &i.Value, &i.Status, &i.AudienceID, &i.AudienceName, &i.VKAdGroupID, &i.VKBannerID, &i.VideoObject, &i.SourceVideoObject, &i.NameTextTemplate, &i.SurnameTextTemplate); err != nil {
			return nil, err
		}
		out = append(out, &i)
	}
	return out, rows.Err()
}
