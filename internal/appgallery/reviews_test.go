package appgallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReviewsQueryAndParsing(t *testing.T) {
	var q url.Values
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != reviewsPath {
			t.Errorf("path = %s", r.URL.Path)
		}
		q = r.URL.Query()
		// The live API answers hasNext as a number, not as a boolean.
		_, _ = w.Write([]byte(`{"ret":{"rtnCode":0},"data":{"total":2,"hasNext":1,"reviewList":[
			{"reviewId":"r-1","content":"Отличное приложение","rating":5,"dateline":1756713600000,
			 "countryCode":"RU","lang":"ru_RU","versionName":"1.4.2","devReplyState":0,"phoneName":"P60"},
			{"reviewId":222,"content":"Не работает","rating":"1","dateline":1756713700000,"devReplyState":1}
		]}}`))
	})

	begin := time.UnixMilli(1756627200000)
	end := time.UnixMilli(1756713600000)
	page, err := c.Reviews(context.Background(), ReviewsParams{
		AppID:       "118236677",
		Begin:       begin,
		End:         end,
		Countries:   []string{"RU", "KZ"},
		Ratings:     []int{1, 2},
		ReplyStates: []int{0},
		Limit:       50,
	})
	if err != nil {
		t.Fatalf("Reviews: %v", err)
	}
	if page.Total != 2 || !page.HasNext || len(page.Reviews) != 2 {
		t.Fatalf("page = %+v", page)
	}

	first := page.Reviews[0]
	if first.ReviewID.String() != "r-1" || first.Rating.Int() != 5 || first.Answered() {
		t.Fatalf("first review = %+v", first)
	}
	if !first.Time().Equal(time.UnixMilli(1756713600000)) {
		t.Fatalf("time = %s", first.Time())
	}
	second := page.Reviews[1]
	// The API mixes strings and numbers across accounts; both must parse.
	if second.ReviewID.String() != "222" || second.Rating.Int() != 1 || !second.Answered() {
		t.Fatalf("second review = %+v", second)
	}

	if got := q.Get("beginTime"); got != "1756627200000" {
		t.Fatalf("beginTime = %q", got)
	}
	if got := q.Get("countries"); got != "RU,KZ" {
		t.Fatalf("countries = %q", got)
	}
	if got := q.Get("ratings"); got != "1,2" {
		t.Fatalf("ratings = %q", got)
	}
	if got := q.Get("devReplyStates"); got != "0" {
		t.Fatalf("devReplyStates = %q", got)
	}
	if got := q.Get("limit"); got != "50" {
		t.Fatalf("limit = %q", got)
	}
}

func TestReviewsRequiresAWindowAndCountry(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s", r.URL.Path)
	})
	now := time.Now()
	cases := []ReviewsParams{
		{Begin: now, End: now, Countries: []string{"RU"}},
		{AppID: "1", End: now, Countries: []string{"RU"}},
		{AppID: "1", Begin: now, End: now},
	}
	for i, p := range cases {
		if _, err := c.Reviews(context.Background(), p); err == nil {
			t.Fatalf("case %d: want a validation error", i)
		}
	}
}

func TestReply(t *testing.T) {
	var body ReplyParams
	var requestID string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != reviewsPath {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		requestID = r.Header.Get("requestId")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"ret":{"rtnCode":0,"rtnDesc":"success"}}`))
	})

	err := c.Reply(context.Background(), ReplyParams{
		AppID:       "118236677",
		ReviewID:    "r-1",
		Content:     "Спасибо, поправили в 1.4.2",
		Lang:        "ru_RU",
		CountryCode: "RU",
	})
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if body.ReviewID != "r-1" || body.Content != "Спасибо, поправили в 1.4.2" {
		t.Fatalf("body = %+v", body)
	}
	if len(requestID) == 0 || len(requestID) > 64 {
		t.Fatalf("requestId = %q", requestID)
	}
}

func TestReplyValidation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s", r.URL.Path)
	})
	full := ReplyParams{AppID: "1", ReviewID: "r", Content: "text", Lang: "ru_RU", CountryCode: "RU"}

	cases := map[string]func(p ReplyParams) ReplyParams{
		"no app":       func(p ReplyParams) ReplyParams { p.AppID = ""; return p },
		"no review":    func(p ReplyParams) ReplyParams { p.ReviewID = ""; return p },
		"empty text":   func(p ReplyParams) ReplyParams { p.Content = "   "; return p },
		"no lang":      func(p ReplyParams) ReplyParams { p.Lang = ""; return p },
		"no country":   func(p ReplyParams) ReplyParams { p.CountryCode = ""; return p },
		"both replies": func(p ReplyParams) ReplyParams { p.ToReplyID = "a"; p.UpdateReplyID = "b"; return p },
	}
	for name, mutate := range cases {
		if err := c.Reply(context.Background(), mutate(full)); err == nil {
			t.Fatalf("%s: want a validation error", name)
		}
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate request id %q", id)
		}
		if strings.TrimSpace(id) == "" {
			t.Fatal("empty request id")
		}
		seen[id] = true
	}
}
