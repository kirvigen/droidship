package rustore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestCreateDraft(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/public/v1/application/com.gdebenz.win/version" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("body decode: %v", err)
		}
		if body["whatsNew"] != "Новые лимиты" || body["publishType"] != "MANUAL" {
			t.Errorf("body = %v", body)
		}
		for _, absent := range []string{"appName", "partialValue", "publishDateTime", "moderInfo"} {
			if _, ok := body[absent]; ok {
				t.Errorf("unset field %q must be omitted, body = %v", absent, body)
			}
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":243242,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	id, err := c.CreateDraft(t.Context(), "com.gdebenz.win", DraftParams{
		WhatsNew:    "Новые лимиты",
		PublishType: "MANUAL",
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if id != 243242 {
		t.Errorf("versionId = %d, want 243242", id)
	}
}

func TestCreateDraftObjectBody(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":"OK","message":null,"body":{"versionId":777},"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	id, err := c.CreateDraft(t.Context(), "com.gdebenz.win", DraftParams{})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if id != 777 {
		t.Errorf("versionId = %d, want 777", id)
	}
}

func TestExistingDraftID(t *testing.T) {
	cases := []struct {
		msg string
		id  int64
		ok  bool
	}{
		{"You already have created draft version with ID = 3489561", 3489561, true},
		{"Уже есть черновик версии с ID 123", 123, true},
		{"Something went wrong", 0, false},
	}
	for _, tc := range cases {
		id, ok := ExistingDraftID(&APIError{HTTPStatus: 400, Code: "ERROR", Message: tc.msg})
		if id != tc.id || ok != tc.ok {
			t.Errorf("ExistingDraftID(%q) = %d,%v want %d,%v", tc.msg, id, ok, tc.id, tc.ok)
		}
	}
	if _, ok := ExistingDraftID(fmt.Errorf("network down")); ok {
		t.Error("non-API error must not yield a draft id")
	}
}

func TestCommit(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/public/v1/application/com.gdebenz.win/version/243242/commit" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("priorityUpdate"); got != "3" {
			t.Errorf("priorityUpdate = %q, want 3", got)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if err := c.Commit(t.Context(), "com.gdebenz.win", 243242, 3); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestPublishAndDeleteAndSettings(t *testing.T) {
	key := testKey(t)
	var gotPublish, gotDelete, gotSettings bool
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/public/v1/application/p/version/7/publish":
			gotPublish = true
		case r.Method == http.MethodDelete && r.URL.Path == "/public/v1/application/p/version/7":
			gotDelete = true
		case r.Method == http.MethodPost && r.URL.Path == "/public/v1/application/p/version/7/publish-settings":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["partialValue"] != float64(25) {
				t.Errorf("settings body = %v", body)
			}
			if _, ok := body["publishType"]; ok {
				t.Errorf("unset publishType must be omitted, body = %v", body)
			}
			gotSettings = true
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if err := c.Publish(t.Context(), "p", 7); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := c.DeleteDraft(t.Context(), "p", 7); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if err := c.UpdatePublishSettings(t.Context(), "p", 7, PublishSettings{PartialValue: 25}); err != nil {
		t.Fatalf("UpdatePublishSettings: %v", err)
	}
	if !gotPublish || !gotDelete || !gotSettings {
		t.Errorf("endpoints hit: publish=%v delete=%v settings=%v", gotPublish, gotDelete, gotSettings)
	}
}
