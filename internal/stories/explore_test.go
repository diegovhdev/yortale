package stories

import (
	"strings"
	"testing"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"
)

func TestValidateChapterMinWords(t *testing.T) {
	short := strings.Repeat("palabra ", minPublishWords-1)
	enough := strings.Repeat("palabra ", minPublishWords)
	tests := []struct {
		name    string
		in      ChapterInput
		wantErr bool
	}{
		{"borrador corto", ChapterInput{Title: "Uno", Content: "Apenas empieza."}, false},
		{"publicar corto", ChapterInput{Title: "Uno", Content: short, Publish: true}, true},
		{"publicar justo el mínimo", ChapterInput{Title: "Uno", Content: enough, Publish: true}, false},
	}
	for _, tt := range tests {
		_, err := validateChapter(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: error = %v, se esperaba error: %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestPeriodStart(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		period string
		want   time.Time
	}{
		{"", time.Time{}},
		{"siglo", time.Time{}},
		{PeriodWeek, time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)},
		{PeriodMonth, now.AddDate(0, -1, 0)},
		{Period3Months, now.AddDate(0, -3, 0)},
		{PeriodYear, time.Date(2025, 3, 31, 12, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		if got := periodStart(tt.period, now); !got.Equal(tt.want) {
			t.Errorf("periodStart(%q) = %v, se esperaba %v", tt.period, got, tt.want)
		}
	}
	// Cada periodo que se ofrece en el formulario debe traducirse a una fecha.
	for _, o := range DatePeriods {
		if o.Value != "" && periodStart(o.Value, now).IsZero() {
			t.Errorf("el periodo %q no tiene fecha de inicio", o.Value)
		}
	}
}

func TestExploreViewPageURL(t *testing.T) {
	v := ExploreView{Updated: PeriodMonth, Published: PeriodYear, Sort: SortOldest}
	want := "/stories?page=2&published=year&sort=oldest&updated=month"
	if got := v.PageURL(2); got != want {
		t.Errorf("PageURL = %q, se esperaba %q", got, want)
	}
	if got := (ExploreView{}).PageURL(1); got != "/stories" {
		t.Errorf("PageURL sin filtros = %q", got)
	}
}

func TestValidateSynopsisMinWords(t *testing.T) {
	svc := &Service{}
	short := strings.Repeat("palabra ", minSynopsisWords-1)
	enough := strings.Repeat("palabra ", minSynopsisWords)
	tests := []struct {
		name     string
		status   string
		synopsis string
		wantErr  bool
	}{
		{"borrador sin sinopsis", StatusDraft, "  ", false},
		{"borrador corta", StatusDraft, short, false},
		{"publicada sin sinopsis", "ongoing", "", true},
		{"publicada corta", "ongoing", short, true},
		{"publicada justo el mínimo", "completed", enough, false},
	}
	for _, tt := range tests {
		_, err := svc.validateStory(t.Context(), StoryInput{Status: tt.status, Synopsis: tt.synopsis})
		verrs, _ := err.(auth.ValidationErrors)
		if got := verrs["synopsis"] != ""; got != tt.wantErr {
			t.Errorf("%s: error de sinopsis = %q, se esperaba error: %v", tt.name, verrs["synopsis"], tt.wantErr)
		}
	}
}

func TestDowngradeIfBlocked(t *testing.T) {
	short := Story{Status: StatusDraft, Synopsis: "Muy corta."}
	tests := []struct {
		name        string
		story       Story
		publish     bool
		wantPublish bool
	}{
		{"guardar sin publicar", short, false, false},
		{"publicar el primero con sinopsis corta", short, true, false},
		{"publicar con sinopsis suficiente", Story{Status: StatusDraft, Synopsis: strings.Repeat("palabra ", minSynopsisWords)}, true, true},
		// Una historia ya publicada no vuelve a comprobarse aquí: su formulario ya lo exige.
		{"historia ya publicada", Story{Status: "ongoing", Synopsis: "Antigua."}, true, true},
	}
	for _, tt := range tests {
		p := ChapterParams{Publish: tt.publish}
		downgradeIfBlocked(tt.story, &p)
		if p.Publish != tt.wantPublish {
			t.Errorf("%s: Publish = %v, se esperaba %v", tt.name, p.Publish, tt.wantPublish)
		}
	}
}

func TestCanMoveChapter(t *testing.T) {
	chapters := []Chapter{{IsPublished: true}, {IsPublished: true}, {}, {}}
	tests := []struct {
		i, delta int
		want     bool
	}{
		{0, -1, false}, // el primero no sube
		{0, 1, true},
		{1, 1, false},  // último publicado: no baja entre los borradores
		{2, -1, false}, // primer borrador: no sube entre los publicados
		{2, 1, true},
		{3, 1, false}, // el último no baja
	}
	for _, tt := range tests {
		if got := canMoveChapter(chapters, tt.i, tt.delta); got != tt.want {
			t.Errorf("canMoveChapter(%d, %d) = %v, se esperaba %v", tt.i, tt.delta, got, tt.want)
		}
	}
}
