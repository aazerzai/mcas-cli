package mcas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Client is a synchronous MCAS client. It holds no long-lived network
// connections beyond an in-memory cookie jar, so it's cheap to create fresh
// per CLI invocation.
type Client struct {
	http    *http.Client
	baseURL string
	creds   Credentials
	session Session
}

// New creates a Client for the given credentials.
func New(creds Credentials) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:    &http.Client{Jar: jar, Timeout: 30 * time.Second},
		baseURL: baseURL,
		creds:   creds,
	}
}

// Session returns the pupil context captured by the last successful login.
func (c *Client) Session() Session { return c.session }

func (c *Client) newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// Login performs the WebForms login and captures the pupil context.
func (c *Client) Login() error {
	loginURL := c.baseURL + loginPath

	getReq, err := c.newRequest(http.MethodGet, loginURL, nil)
	if err != nil {
		return &APIError{Message: err.Error()}
	}
	resp, err := c.http.Do(getReq)
	if err != nil {
		return &APIError{Message: "fetching login page: " + err.Error()}
	}
	page, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return &APIError{Message: "reading login page: " + err.Error()}
	}

	form := hiddenFields(string(page))
	if len(form) == 0 {
		return &APIError{Message: "Login page had no hidden fields; layout may have changed"}
	}
	form["EmailTextBox"] = c.creds.Email
	form["PasswordTextBox"] = c.creds.Password

	values := url.Values{}
	for k, v := range form {
		values.Set(k, v)
	}

	postReq, err := c.newRequest(http.MethodPost, loginURL, strings.NewReader(values.Encode()))
	if err != nil {
		return &APIError{Message: err.Error()}
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postResp, err := c.http.Do(postReq)
	if err != nil {
		return &APIError{Message: "posting login form: " + err.Error()}
	}
	defer postResp.Body.Close()
	respBody, err := io.ReadAll(postResp.Body)
	if err != nil {
		return &APIError{Message: "reading login response: " + err.Error()}
	}

	finalURL := ""
	if postResp.Request != nil && postResp.Request.URL != nil {
		finalURL = postResp.Request.URL.String()
	}
	if !strings.Contains(finalURL, dashboardMarker) {
		return &AuthError{Message: describeLoginFailure(finalURL, string(respBody))}
	}

	ctx := readDashboardContext(string(respBody))
	c.session = Session{
		StudentID:   ctx.StudentID,
		SchoolID:    ctx.SchoolID,
		SchoolName:  ctx.SchoolName,
		StudentName: ctx.StudentName,
	}
	return nil
}

// describeLoginFailure explains why a login did not reach the dashboard.
//
// MCAS answers every outcome with HTTP 200 and simply renders a different
// page, so the landing URL is the only signal. Reporting "check your
// password" for all of them is misleading: a correct password can still be
// held short by a terms-and-conditions page or a security-question setup,
// which is exactly what a school can force after a password reset.
func describeLoginFailure(finalURL, body string) string {
	interstitials := []struct {
		page    string
		message string
	}{
		{"MCSTermsAndConditions", "MyChildAtSchool is asking you to accept its terms and conditions. Sign in at mychildatschool.com once and accept them, then retry."},
		{"MCSParentSecurityQuestions", "MyChildAtSchool is asking you to set security questions. Sign in at mychildatschool.com once and complete them, then retry."},
		{"MCSAccountSettings", "MyChildAtSchool redirected to account settings, which usually means it wants something completed on the account. Sign in there once, then retry."},
	}
	lowerURL := strings.ToLower(finalURL)
	for _, i := range interstitials {
		if strings.Contains(lowerURL, strings.ToLower(i.page)) {
			return i.message
		}
	}

	if text := findErrorText(body); text != "" {
		return "Login rejected by MyChildAtSchool: " + text
	}

	landed := finalURL
	if idx := strings.LastIndex(landed, "/"); idx >= 0 {
		landed = landed[idx+1:]
	}
	if idx := strings.Index(landed, "?"); idx >= 0 {
		landed = landed[:idx]
	}
	if landed != "" && landed != "MCSParentLogin" {
		return fmt.Sprintf("Login did not reach the dashboard - MyChildAtSchool sent us to '%s'. Sign in at mychildatschool.com to see what it wants.", landed)
	}
	return "Login failed - check the email address and password"
}

// ensureSession re-logs in if no session has been established yet.
func (c *Client) ensureSession() error {
	if c.session.StudentID == 0 {
		return c.Login()
	}
	return nil
}

// get calls a backend api/v1 path through the portal's proxy and returns the
// decoded "d" payload: either the raw JSON text or an HTML fragment,
// depending on the route. Returns "" if MCAS reported no data or an unknown
// route.
//
// An expired session is not reported as a 401 or a redirect to the login
// page - the proxy raises server-side and returns HTTP 500 with a generic
// ASP.NET error body. That is indistinguishable from a real server fault, so
// the only reliable response is to log in again and retry once: if the
// retry succeeds the session had simply lapsed, and if the fresh login
// fails the credentials are genuinely wrong.
func (c *Client) get(path string) (string, error) {
	return c.getRetry(path, true)
}

func (c *Client) getRetry(path string, allowRetry bool) (string, error) {
	payload, err := json.Marshal(map[string]string{"url": path, "schoolID": "", "contactID": ""})
	if err != nil {
		return "", &APIError{Message: err.Error()}
	}
	req, err := c.newRequest(http.MethodPost, c.baseURL+proxyPath, bytes.NewReader(payload))
	if err != nil {
		return "", &APIError{Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", &APIError{Message: fmt.Sprintf("%s -> %s", path, err.Error())}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &APIError{Message: err.Error()}
	}

	peek := string(data)
	if len(peek) > 200 {
		peek = peek[:200]
	}
	if resp.StatusCode == http.StatusUnauthorized || strings.Contains(peek, dashboardMarker) {
		return "", &AuthError{Message: "Session expired"}
	}
	if resp.StatusCode == http.StatusInternalServerError && allowRetry {
		if err := c.Login(); err != nil {
			return "", err // AuthError if credentials no longer work
		}
		return c.getRetry(path, false)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Message: fmt.Sprintf("%s -> HTTP %d", path, resp.StatusCode)}
	}

	var wrapper struct {
		D *string `json:"d"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", &APIError{Message: "invalid proxy response: " + err.Error()}
	}
	if wrapper.D == nil || *wrapper.D == "" {
		return "", nil
	}
	if strings.HasPrefix(*wrapper.D, notFoundMarker) {
		// Wrong path, or the wrong number of path parameters.
		return "", nil
	}
	return *wrapper.D, nil
}
