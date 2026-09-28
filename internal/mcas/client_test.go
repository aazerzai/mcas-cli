package mcas

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := New(Credentials{Email: "parent@example.com", Password: "hunter2"})
	c.baseURL = server.URL
	return c
}

// TestLoginPostsHiddenFieldsAndReadsContext mirrors
// test_login_posts_hidden_fields_and_reads_context in the reference repo's
// test_api.py: a successful login lands on a URL containing the dashboard
// marker, and the WebForms hidden fields must be echoed back.
func TestLoginPostsHiddenFieldsAndReadsContext(t *testing.T) {
	var postBody string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == loginPath:
			w.Write([]byte(loginHTML))
		case r.Method == http.MethodPost && r.URL.Path == loginPath:
			body, _ := io.ReadAll(r.Body)
			postBody = string(body)
			http.Redirect(w, r, "/MCAS/MCSDashboardPage", http.StatusFound)
		case r.URL.Path == "/MCAS/MCSDashboardPage":
			w.Write([]byte(dashboardHTML))
		default:
			http.NotFound(w, r)
		}
	})

	if err := client.Login(); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !strings.Contains(postBody, "__VIEWSTATE=abc123") {
		t.Errorf("post body missing __VIEWSTATE: %s", postBody)
	}
	if !strings.Contains(postBody, "__EVENTVALIDATION=xyz789") {
		t.Errorf("post body missing __EVENTVALIDATION: %s", postBody)
	}
	if !strings.Contains(postBody, "EmailTextBox=parent%40example.com") {
		t.Errorf("post body missing EmailTextBox: %s", postBody)
	}

	session := client.Session()
	if session.StudentID != 99999 || session.SchoolID != 88888 {
		t.Errorf("session = %+v, want StudentID=99999 SchoolID=88888", session)
	}
	if session.SchoolName != "Example High School" {
		t.Errorf("SchoolName = %q", session.SchoolName)
	}
	if session.StudentName != "Alex Smith" {
		t.Errorf("StudentName = %q, want Alex Smith", session.StudentName)
	}
}

// TestLoginFailureRaisesAuthError mirrors test_login_failure_raises_auth_error:
// MCAS re-renders the login page rather than returning an error status.
func TestLoginFailureRaisesAuthError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(loginHTML))
	})
	err := client.Login()
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("Login() error = %v (%T), want *AuthError", err, err)
	}
}

// TestExpiredSessionRelogsAndRetries mirrors
// test_expired_session_relogins_and_retries: a lapsed session shows up as
// HTTP 500, not a 401 or a login redirect, and the client should recover by
// logging in again and retrying once.
func TestExpiredSessionRelogsAndRetries(t *testing.T) {
	proxyCalls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == proxyPath:
			proxyCalls++
			if proxyCalls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("server error"))
				return
			}
			w.Write([]byte(`{"d": "{\"ok\": true}"}`))
		case r.Method == http.MethodGet && r.URL.Path == loginPath:
			w.Write([]byte(loginHTML))
		case r.Method == http.MethodPost && r.URL.Path == loginPath:
			http.Redirect(w, r, "/MCAS/MCSDashboardPage", http.StatusFound)
		case r.URL.Path == "/MCAS/MCSDashboardPage":
			w.Write([]byte(dashboardHTML))
		default:
			http.NotFound(w, r)
		}
	})

	raw, err := client.get("api/v1/anything")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if raw != `{"ok": true}` {
		t.Errorf("get() = %q", raw)
	}
	if client.Session().StudentID != 99999 {
		t.Errorf("client did not re-authenticate: session = %+v", client.Session())
	}
}

// TestExpiredSessionWithBadPasswordRaisesAuthError mirrors
// test_expired_session_with_bad_password_raises_auth_error: after a
// password change the retry must fail as auth, not as a server fault, or
// the client would retry forever instead of asking for the new password.
func TestExpiredSessionWithBadPasswordRaisesAuthError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == proxyPath:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
		case r.Method == http.MethodGet && r.URL.Path == loginPath:
			w.Write([]byte(loginHTML))
		case r.Method == http.MethodPost && r.URL.Path == loginPath:
			w.Write([]byte(loginHTML))
		default:
			http.NotFound(w, r)
		}
	})

	_, err := client.get("api/v1/anything")
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("get() error = %v (%T), want *AuthError", err, err)
	}
}

// TestPersistentServerErrorIsNotMisreportedAsAuth mirrors
// test_persistent_500_after_relogin_is_a_server_error: a genuine server
// fault should not be misreported as bad credentials.
func TestPersistentServerErrorIsNotMisreportedAsAuth(t *testing.T) {
	proxyCalls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == proxyPath:
			proxyCalls++
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
		case r.Method == http.MethodGet && r.URL.Path == loginPath:
			w.Write([]byte(loginHTML))
		case r.Method == http.MethodPost && r.URL.Path == loginPath:
			http.Redirect(w, r, "/MCAS/MCSDashboardPage", http.StatusFound)
		case r.URL.Path == "/MCAS/MCSDashboardPage":
			w.Write([]byte(dashboardHTML))
		default:
			http.NotFound(w, r)
		}
	})

	_, err := client.get("api/v1/anything")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("get() error = %v (%T), want *APIError", err, err)
	}
}

// TestProxyDecodesPayloads mirrors
// test_proxy_decodes_double_encoded_payloads: the "d" wrapper can carry
// JSON text, an HTML fragment, or the "Error 404" marker for an unknown
// route, none of which surface as a transport-level error.
func TestProxyDecodesPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"json", `{"d": "{\"Table\": [{\"a\": 1}]}"}`, `{"Table": [{"a": 1}]}`},
		{"html", `{"d": "<table><tr><td>x</td></tr></table>"}`, "<table><tr><td>x</td></tr></table>"},
		{"empty string", `{"d": ""}`, ""},
		{"null", `{"d": null}`, ""},
		{"not found", `{"d": "Error 404 - Requested API call reference Not Found."}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.body))
			})
			got, err := client.get("api/v1/anything")
			if err != nil {
				t.Fatalf("get() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("get() = %q, want %q", got, tt.want)
			}
		})
	}
}
