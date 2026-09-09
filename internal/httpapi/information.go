package httpapi

import (
	"context"
	"html/template"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SupportRequest struct {
	ID      uuid.UUID
	Kind    string
	Email   string
	Message string
}

func mountInformationPages(r chi.Router, deps Dependencies) {
	for path, page := range map[string]infoPage{
		"/": {Title: "Learning English", Paragraphs: []string{
			"An independent, audio-first English learning app. Listen to an article, retell it in your own words, review vocabulary and practise grammar — at your own pace.",
			"Learning works without an account. Each article retains its source attribution. Browse our help and privacy pages below, or send feedback about the app or its content.",
		}},
		"/privacy": {Title: "Privacy / 隐私说明", Paragraphs: []string{
			"Updated 2026-09-10. Learning English is an independent English learning app. You can browse and learn without an account.",
			"Anonymous learning data — saved lessons, listening progress, words, grammar practice and your recordings — stays on your device. Removing the app removes that data. You can export a learning backup from Account → Data & Privacy and restore it on another installation. Backups contain your recordings and learning data, but never account tokens. Keep the exported file private.",
			"When you choose to sign in, the service stores your account identifier and syncs your learning data, private recordings, transcripts and practice feedback. These are used for app functionality and learning recommendations. We do not include advertising or third-party tracking SDKs, sell learning data, or use your recordings for advertising.",
			"Microphone permission is requested only when you record. Speech recognition is optional and performed on the device when supported; if unavailable, recording and playback still work. Camera, contacts and location access are not requested.",
			"The API and hosting provider process network addresses and request logs to deliver content and diagnose failures. Feedback submitted through Support is stored with the contact address you voluntarily provide. Do not include passwords or recordings in feedback.",
			"You can delete individual words and recordings in the app. Account → Data & Privacy lets you erase local learning data, or delete a signed-in account and its stored learning data and recordings. Deleting local data does not delete cloud data. Daily production database backups use a 14-day retention setting. Disaster-recovery copies are handled separately; subsequent deletion requests must be reapplied before any restored service resumes.",
			"For privacy requests or content-rights concerns, use the Support form linked below. Source attribution and links are included with each article. Opening a source link is subject to that website's own privacy practices.",
		}},
		"/support": {Title: "Help & Feedback / 帮助与反馈", Support: true, Paragraphs: []string{
			"Browse Home or Explore, select an audio lesson and use the floating player. Tap the title to return to the article. Long-press a word to look it up or add it to Word Book. Recording, vocabulary review and grammar practice work without signing in.",
			"Audio streams over the network; it is not downloaded for offline listening. If playback or loading fails, reconnect and try again. Your saved learning data remains on the device. Development builds can select Internal or Production in Account → Development → Environment.",
			"Submit app problems, privacy requests or content-rights reports here. Include the article title or source URL where relevant. A contact email is optional, but is needed if you would like a reply. Feedback is visible only to the operator; it is not posted publicly.",
		}},
	} {
		r.Get(path, func(w http.ResponseWriter, req *http.Request) { renderInfoPage(w, http.StatusOK, page) })
	}
	r.With(newIPRateLimiter(5, 3).middleware).Post("/support", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Feedback is too long.", 400)
			return
		}
		input := SupportRequest{ID: uuid.New(), Kind: r.FormValue("kind"), Email: strings.TrimSpace(r.FormValue("email")), Message: strings.TrimSpace(r.FormValue("message"))}
		if len(input.Message) < 10 || len(input.Message) > 8000 || len(input.Email) > 254 || (input.Kind != "problem" && input.Kind != "privacy" && input.Kind != "content") {
			http.Error(w, "Please choose a topic and enter 10–8000 characters.", 400)
			return
		}
		if input.Email != "" {
			address, err := mail.ParseAddress(input.Email)
			if err != nil || address.Address != input.Email {
				http.Error(w, "Please check your email address.", 400)
				return
			}
		}
		if deps.Support == nil {
			http.Error(w, "Feedback is temporarily unavailable. Please try again later.", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := deps.Support(ctx, input); err != nil {
			http.Error(w, "Feedback could not be saved. Please try again later.", 503)
			return
		}
		renderInfoPage(w, http.StatusCreated, infoPage{Title: "Feedback received / 已收到反馈", Paragraphs: []string{"Your reference: " + input.ID.String(), "Your message has been saved for review. Keep this reference if you contact us again."}})
	})
}

type infoPage struct {
	Title      string
	Paragraphs []string
	Support    bool
}

var informationTemplate = template.Must(template.New("information").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · Learning English</title><style>body{font:17px/1.65 system-ui,sans-serif;background:#f6f6f8;color:#222;margin:0}main{max-width:720px;margin:40px auto;padding:28px;background:white;border-radius:22px}h1{line-height:1.2}a{color:#b92151}nav{display:flex;gap:22px}label{display:block;margin-top:18px}input,select,textarea,button{font:inherit;box-sizing:border-box;width:100%;padding:12px;border:1px solid #aaa;border-radius:10px}textarea{min-height:180px}button{margin-top:20px;background:#b92151;color:white;border:0}small{color:#666}@media(max-width:760px){main{margin:16px;padding:22px}}</style></head><body><main><small>LEARNING ENGLISH</small><h1>{{.Title}}</h1>{{range .Paragraphs}}<p>{{.}}</p>{{end}}{{if .Support}}<form action="/support" method="post"><label>Topic / 类型<select name="kind"><option value="problem">App problem / 使用问题</option><option value="privacy">Privacy / 隐私请求</option><option value="content">Content rights / 内容版权</option></select></label><label>Contact email (optional) / 联系邮箱（可选）<input type="email" name="email" maxlength="254" autocomplete="email"></label><label>Message / 说明<textarea name="message" minlength="10" maxlength="8000" required></textarea></label><button type="submit">Send feedback / 提交反馈</button></form>{{end}}<hr><nav><a href="/privacy">Privacy / 隐私</a><a href="/support">Support / 帮助</a></nav></main></body></html>`))

func renderInfoPage(w http.ResponseWriter, status int, page infoPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_ = informationTemplate.Execute(w, page)
}
