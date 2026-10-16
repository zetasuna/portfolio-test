package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/example/portfolio-crud/internal/model"
	"github.com/example/portfolio-crud/internal/repository"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var ErrValidation = errors.New("validation failed")

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string  { return e.Field + ": " + e.Message }
func invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }

type ProjectService struct{ Repo *repository.ProjectRepository }

func ValidateProject(in model.ProjectInput) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(in.Title)); n < 1 || n > 120 {
		return invalid("title", "Title must contain 1–120 characters")
	}
	if !slugPattern.MatchString(in.Slug) || len(in.Slug) > 100 {
		return invalid("slug", "Use lowercase letters, numbers and single hyphens")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(in.Summary)); n < 1 || n > 250 {
		return invalid("summary", "Summary must contain 1–250 characters")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(in.Description)); n < 1 || n > 10000 {
		return invalid("description", "Description must contain 1–10,000 characters")
	}
	if len(in.Technologies) > 20 {
		return invalid("technologies", "Use at most 20 technologies")
	}
	for _, tech := range in.Technologies {
		if utf8.RuneCountInString(tech) > 40 {
			return invalid("technologies", "Each technology must be at most 40 characters")
		}
	}
	if in.Status != model.StatusDraft && in.Status != model.StatusPublished {
		return invalid("status", "Status must be draft or published")
	}
	for field, raw := range map[string]string{"github_url": in.GitHubURL, "demo_url": in.DemoURL} {
		if len(raw) > 2048 {
			return invalid(field, "URL must be at most 2048 bytes")
		}
		if raw == "" {
			continue
		}
		u, err := url.ParseRequestURI(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return invalid(field, "Use a valid HTTPS URL without embedded credentials")
		}
	}
	return nil
}
func (s *ProjectService) Create(ctx context.Context, in model.ProjectInput) (*model.Project, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	in.Description = strings.TrimSpace(in.Description)
	in.Technologies = repository.CleanTechnologies(in.Technologies)
	if err := ValidateProject(in); err != nil {
		return nil, err
	}
	p, err := s.Repo.Create(ctx, in)
	if errors.Is(err, repository.ErrSlugConflict) {
		return nil, fmt.Errorf("%w: slug already exists", repository.ErrSlugConflict)
	}
	return p, err
}
func (s *ProjectService) Update(ctx context.Context, id string, in model.ProjectInput) (*model.Project, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	in.Description = strings.TrimSpace(in.Description)
	in.Technologies = repository.CleanTechnologies(in.Technologies)
	if err := ValidateProject(in); err != nil {
		return nil, err
	}
	return s.Repo.Update(ctx, id, in)
}
