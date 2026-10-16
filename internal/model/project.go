package model

import "time"

type ProjectStatus string

const (
	StatusDraft     ProjectStatus = "draft"
	StatusPublished ProjectStatus = "published"
)

type Project struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Slug          string        `json:"slug"`
	Summary       string        `json:"summary"`
	Description   string        `json:"description"`
	ThumbnailPath string        `json:"thumbnail_path"`
	Technologies  []string      `json:"technologies"`
	GitHubURL     string        `json:"github_url"`
	DemoURL       string        `json:"demo_url"`
	Status        ProjectStatus `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type ProjectInput struct {
	Title         string
	Slug          string
	Summary       string
	Description   string
	Technologies  []string
	GitHubURL     string
	DemoURL       string
	Status        ProjectStatus
	ThumbnailPath string
}

type ProjectStats struct {
	Total     int
	Published int
	Draft     int
}
