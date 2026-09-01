package voa

import (
	"os"
	"testing"

	"github.com/oldj/voa-learning-app/backend/internal/catalog"
)

func TestArticleParserParsesLessonFixture(t *testing.T) {
	file, err := os.Open("../../../fixtures/voa/lesson.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	content, err := (ArticleParser{}).Parse("https://learningenglish.voanews.com/a/6654462.html", file)
	if err != nil {
		t.Fatal(err)
	}
	if content.Source.ExternalID != "6654462" {
		t.Fatalf("external ID = %q", content.Source.ExternalID)
	}
	if content.Title != "Lesson 1: Who Are You?" || content.Type != catalog.ContentTypeLesson {
		t.Fatalf("unexpected content: %+v", content)
	}
	if content.Level != catalog.LevelBeginning {
		t.Fatalf("level=%q", content.Level)
	}
	if len(content.Body) != 3 {
		t.Fatalf("blocks = %d", len(content.Body))
	}
	if len(content.Assets) != 3 {
		t.Fatalf("assets = %d: %+v", len(content.Assets), content.Assets)
	}
}

func TestArticleParserRequiresTitle(t *testing.T) {
	file, err := os.Open("../../../fixtures/voa/missing-title.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := (ArticleParser{}).Parse("https://learningenglish.voanews.com/a/1.html", file); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestArticleParserRejectsArticleURLServingLivePage(t *testing.T) {
	file, err := os.Open("../../../fixtures/voa/live/latest-8187538.html")
	if os.IsNotExist(err) {
		t.Skip("run scripts/capture_fixtures.sh")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := (ArticleParser{}).Parse("https://learningenglish.voanews.com/a/8187538.html", file); err == nil {
		t.Fatal("expected structured-data mismatch")
	}
}
