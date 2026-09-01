package catalog

import "time"

type ContentType string

const (
	ContentTypeArticle ContentType = "article"
	ContentTypeLesson  ContentType = "lesson"
	ContentTypePodcast ContentType = "podcast"
	ContentTypeVideo   ContentType = "video"
	ContentTypeQuiz    ContentType = "quiz"
)

type Level string

type RightsStatus string

const (
	RightsReviewRequired       RightsStatus = "review_required"
	RightsPublicDomainVerified RightsStatus = "public_domain_verified"
	RightsThirdPartyRestricted RightsStatus = "third_party_restricted"
)

const (
	LevelBeginning    Level = "beginning"
	LevelIntermediate Level = "intermediate"
	LevelAdvanced     Level = "advanced"
)

type Block struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Level int    `json:"level,omitempty"`
}

type Asset struct {
	Kind         string
	SourceURL    string
	QualityLabel string
	MIMEType     string
	ByteSize     int64
}

type SourceRef struct {
	ExternalID   string
	CanonicalURL string
}

type Content struct {
	Source       SourceRef
	Type         ContentType
	Title        string
	SeriesTitle  string
	Level        Level
	PublishedAt  time.Time
	Body         []Block
	Assets       []Asset
	RightsStatus RightsStatus
	Attribution  string
}
