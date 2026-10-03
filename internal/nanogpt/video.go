package nanogpt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	videoPath       = "/api/generate-video"
	videoStatusPath = "/api/video/status"

	// VideoPollInterval is the cadence nano-gpt's own polling example uses.
	VideoPollInterval = 5 * time.Second

	// VideoWaitBudget is how long one call waits for a video, which is that example's whole
	// window of 120 polls five seconds apart.
	VideoWaitBudget = 10 * time.Minute

	StateQueued     = "queued"
	StateProcessing = "processing"
	StateCompleted  = "completed"
	StateFailed     = "failed"
	StateCancelled  = "cancelled"
)

// VideoRequest carries the fields this server exposes. nano-gpt takes some sixty, most of them
// legacy duplicates or model-specific; these are the ones the documented models share.
type VideoRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`

	NegativePrompt string `json:"negative_prompt,omitempty"`
	Duration       string `json:"duration,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Resolution     string `json:"resolution,omitempty"`
	Mode           string `json:"mode,omitempty"`

	// Pointers keep an unset toggle distinguishable from an explicit false.
	Seed          *int  `json:"seed,omitempty"`
	GenerateAudio *bool `json:"generateAudio,omitempty"`

	ImageURL string `json:"imageUrl,omitempty"`
	VideoURL string `json:"videoUrl,omitempty"`
}

type VideoRun struct {
	RunID         string  `json:"runId"`
	Status        string  `json:"status"`
	Model         string  `json:"model"`
	Cost          float64 `json:"cost"`
	PaymentSource string  `json:"paymentSource"`
}

type Video struct {
	RunID   string
	Model   string
	URL     string
	CostUSD float64
}

// VideoStatus covers both shapes nano-gpt documents for this endpoint: the flat object in its
// response schema, and the data-wrapped object with uppercase states that its examples show.
type VideoStatus struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
	VideoURL  string `json:"videoUrl"`
	Error     string `json:"error"`

	Model string `json:"model"`
	Data  struct {
		Status string `json:"status"`
		Output struct {
			Video struct {
				URL string `json:"url"`
			} `json:"video"`
		} `json:"output"`
		Cost              float64 `json:"cost"`
		Error             string  `json:"error"`
		UserFriendlyError string  `json:"userFriendlyError"`
	} `json:"data"`
}

// GenerateVideo submits a job and waits it out, so a caller gets a finished asset rather than a
// run id to poll.
func (c *Client) GenerateVideo(ctx context.Context, req VideoRequest) (*Video, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	run, err := c.submitVideo(ctx, req)
	if err != nil {
		return nil, err
	}

	return c.waitForVideo(ctx, run)
}

func (c *Client) submitVideo(ctx context.Context, req VideoRequest) (*VideoRun, error) {
	var out VideoRun

	cred := credential{header: "x-api-key", value: c.apiKey}
	if err := c.post(ctx, videoPath, FastTimeout, cred, req, &out); err != nil {
		return nil, err
	}

	if out.RunID == "" {
		return nil, fmt.Errorf("submit %s: the response carries no runId", videoPath)
	}

	if out.Model == "" {
		out.Model = req.Model
	}

	return &out, nil
}

func (c *Client) waitForVideo(ctx context.Context, run *VideoRun) (*Video, error) {
	ctx, cancel := context.WithTimeout(ctx, c.videoWaitBudget)
	defer cancel()

	state := StateQueued

	for {
		if ctx.Err() != nil {
			return nil, waitFailure(ctx, run.RunID, state, c.videoWaitBudget)
		}

		status, err := c.videoStatus(ctx, run.RunID)
		if err != nil {
			// A poll that the wait budget ran out on reports the deadline rather than the
			// transport error it surfaces as, which says nothing about the job itself.
			if ctx.Err() != nil {
				return nil, waitFailure(ctx, run.RunID, state, c.videoWaitBudget)
			}

			return nil, err
		}

		state = status.state()

		switch state {
		case StateCompleted:
			if status.videoURL() == "" {
				return nil, fmt.Errorf("video %s completed without a URL", run.RunID)
			}

			return &Video{
				RunID:   run.RunID,
				Model:   run.Model,
				URL:     status.videoURL(),
				CostUSD: status.Data.Cost,
			}, nil
		case StateFailed, StateCancelled:
			return nil, fmt.Errorf("video %s %s: %s", run.RunID, state, status.failure())
		}

		select {
		case <-ctx.Done():
			return nil, waitFailure(ctx, run.RunID, state, c.videoWaitBudget)
		case <-time.After(c.videoPollInterval):
		}
	}
}

func (c *Client) videoStatus(ctx context.Context, runID string) (*VideoStatus, error) {
	var out VideoStatus

	path := videoStatusPath + "?requestId=" + url.QueryEscape(runID)

	cred := credential{header: "x-api-key", value: c.apiKey}
	if err := c.get(ctx, path, FastTimeout, cred, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func waitFailure(ctx context.Context, runID, state string, budget time.Duration) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("video %s is still %s after %s", runID, state, budget)
	}

	return fmt.Errorf("video %s is still %s: %w", runID, state, ctx.Err())
}

// state reduces the two documented status vocabularies to the lowercase set the unified
// endpoint's schema names.
func (s VideoStatus) state() string {
	state := s.Status
	if state == "" {
		state = s.Data.Status
	}

	switch strings.ToLower(strings.TrimSpace(state)) {
	case "in_queue", "pending":
		return StateQueued
	case "in_progress", "processing", "generating", "delivering":
		return StateProcessing
	case "completed", "succeeded":
		return StateCompleted
	case "canceled":
		return StateCancelled
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

func (s VideoStatus) videoURL() string {
	if s.VideoURL != "" {
		return s.VideoURL
	}

	return s.Data.Output.Video.URL
}

func (s VideoStatus) failure() string {
	for _, candidate := range []string{s.Data.Error, s.Data.UserFriendlyError, s.Error} {
		if candidate != "" {
			return candidate
		}
	}

	return "no reason given"
}

func (r VideoRequest) validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("model is required")
	}

	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}

	return nil
}
