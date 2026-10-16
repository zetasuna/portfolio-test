package service

import (
	"github.com/example/portfolio-crud/internal/model"
	"testing"
)

func validInput() model.ProjectInput {
	return model.ProjectInput{Title: "Portfolio API", Slug: "portfolio-api", Summary: "A clean and testable API.", Description: "Detailed project description.", Technologies: []string{"Go", "PostgreSQL"}, GitHubURL: "https://github.com/example/project", DemoURL: "https://example.com", Status: model.StatusDraft}
}
func TestValidateProjectAcceptsValidInput(t *testing.T) {
	if err := ValidateProject(validInput()); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}
func TestValidateProjectRejectsBadSlug(t *testing.T) {
	in := validInput()
	in.Slug = "Bad slug"
	if err := ValidateProject(in); err == nil {
		t.Fatal("expected invalid slug")
	}
}
func TestValidateProjectRejectsNonHTTPSURL(t *testing.T) {
	in := validInput()
	in.DemoURL = "javascript:alert(1)"
	if err := ValidateProject(in); err == nil {
		t.Fatal("expected URL validation error")
	}
}
func TestValidateProjectRejectsInvalidStatus(t *testing.T) {
	in := validInput()
	in.Status = "private"
	if err := ValidateProject(in); err == nil {
		t.Fatal("expected status validation error")
	}
}
func TestValidateProjectRejectsTooManyTechnologies(t *testing.T) {
	in := validInput()
	in.Technologies = make([]string, 21)
	for i := range in.Technologies {
		in.Technologies[i] = "Go"
	}
	if err := ValidateProject(in); err == nil {
		t.Fatal("expected technology limit error")
	}
}
