package rustore

import (
	"encoding/json"
	"net/http"
	"testing"
)

// routes serves a RuStore fake that answers auth plus the given "METHOD /path" routes.
func routes(t *testing.T, handlers map[string]http.HandlerFunc) *Client {
	t.Helper()
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		h, ok := handlers[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h(w, r)
	})
	return newTestClient(key, "42", srv.URL)
}

func TestCommentsAndFeedbacks(t *testing.T) {
	c := routes(t, map[string]http.HandlerFunc{
		"GET /public/v1/application/com.x/comment": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("size") != "100" || r.URL.Query().Get("page") != "0" {
				t.Errorf("query = %q", r.URL.RawQuery)
			}
			w.Write([]byte(`{"code":"OK","body":[{"commentId":7,"userName":"Даня","appRating":2,
				"commentText":"не грузится","commentDateIso":"2026-04-13T05:03:02.552Z","appVersionName":"1.18"}]}`))
		},
		"GET /public/v1/application/com.x/feedback": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"code":"OK","body":[{"id":"9","commentId":"7","text":"поправили","status":"PUBLISHED"}]}`))
		},
	})
	cs, err := c.Comments(t.Context(), "com.x", 0, 100)
	if err != nil || len(cs) != 1 || cs[0].CommentID != 7 || cs[0].Time().Year() != 2026 {
		t.Fatalf("comments %+v, %v", cs, err)
	}
	fs, err := c.Feedbacks(t.Context(), "com.x", 0, 100)
	if err != nil || len(fs) != 1 || fs[0].CommentID != "7" {
		t.Fatalf("feedbacks %+v, %v", fs, err)
	}
}

func TestAnswer(t *testing.T) {
	c := routes(t, map[string]http.HandlerFunc{
		"POST /public/v1/application/com.x/feedback": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("commentId") != "7" {
				t.Errorf("commentId = %q", r.URL.Query().Get("commentId"))
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["message"] != "спасибо" {
				t.Errorf("message = %q", body["message"])
			}
			w.Write([]byte(`{"code":"OK","body":{"id":748479}}`))
		},
	})
	id, err := c.Answer(t.Context(), "com.x", "7", "спасибо")
	if err != nil || id != 748479 {
		t.Fatalf("id %d, %v", id, err)
	}
}
