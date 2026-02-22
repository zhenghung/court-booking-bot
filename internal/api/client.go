package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client handles HTTP communication with gpropsystems.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	csrfToken  string
}

// NewClient creates a new API client with cookie jar for session management.
func NewClient(baseURL string) *Client {
	jar, _ := cookiejar.New(nil)

	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 10 * time.Second,
		},
	}
}

// Ping makes a GET request to the base URL and returns the HTTP status.
func (c *Client) Ping() (string, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	return resp.Status, nil
}

// Login authenticates with gpropsystems and stores the session cookies.
func (c *Client) Login(email, password string) error {
	// Step 1: GET the login page to extract CSRF token
	csrfToken, err := c.fetchCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to fetch CSRF token: %w", err)
	}
	c.csrfToken = csrfToken
	fmt.Printf("  CSRF token: %s...%s\n", csrfToken[:8], csrfToken[len(csrfToken)-4:])

	// Step 2: POST login credentials
	form := url.Values{}
	form.Set("is_ajax", "1")
	form.Set("email", email)
	form.Set("password", password)
	form.Set("_co6sO0rpsfat", csrfToken)

	req, err := http.NewRequest("POST", c.BaseURL+"/login/login_data_submit", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login returned status %d: %s", resp.StatusCode, string(body))
	}

	// Check if login was successful by looking at the response
	var loginResp map[string]interface{}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("failed to parse login response (got non-JSON, status %d)", resp.StatusCode)
	}

	// Check the status field
	if status, ok := loginResp["status"].(bool); ok && !status {
		msgTitle, _ := loginResp["msg_title"].(string)
		return fmt.Errorf("login failed: %s", msgTitle)
	}

	// Update CSRF token from response if present (server may rotate it)
	if newToken, ok := loginResp["_co6sO0rpsfat"].(string); ok && newToken != "" {
		c.csrfToken = newToken
	}

	fmt.Printf("  Login status: %v\n", loginResp["status"])
	return nil
}

// Timeslot represents a single bookable time slot.
type Timeslot struct {
	Time      string // e.g. "07:00-08:00"
	Available bool   // true if btn-grey (available), false if taken
}

// GetTimeslots fetches available timeslots for a given facility and date.
func (c *Client) GetTimeslots(facilityID, date string) ([]Timeslot, error) {
	form := url.Values{}
	form.Set("is_ajax", "1")
	form.Set("facilityId", facilityID)
	form.Set("bookingDate", date)
	form.Set("_co6sO0rpsfat", c.csrfToken)

	req, err := http.NewRequest("POST", c.BaseURL+"/booking/get_booking_timeslot", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create timeslot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("timeslot request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read timeslot response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("timeslot request returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON to extract the "html" field
	var tsResp map[string]interface{}
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("failed to parse timeslot response: %w", err)
	}

	if status, ok := tsResp["status"].(bool); ok && !status {
		return nil, fmt.Errorf("timeslot request failed (status=false)")
	}

	htmlStr, ok := tsResp["html"].(string)
	if !ok {
		return nil, fmt.Errorf("timeslot response missing 'html' field")
	}

	return parseTimeslotHTML(htmlStr), nil
}

// parseTimeslotHTML extracts timeslots from the HTML returned by the API.
// Available slots: class="btn btn-grey timeslot-block" (no "taken"), with <input> checkbox
// Taken slots:     class="btn btn-grey timeslot-block taken", no <input> checkbox
func parseTimeslotHTML(html string) []Timeslot {
	var slots []Timeslot

	// Match each timeslot div block: captures the class list and the display text (e.g. "07:00 - 08:00")
	divRe := regexp.MustCompile(`class="btn btn-grey timeslot-block([^"]*)"[^>]*>(\d{2}:\d{2})\s*-\s*(\d{2}:\d{2})`)

	matches := divRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		extraClasses := m[1] // e.g. "" or " taken"
		timeRange := m[2] + "-" + m[3]
		taken := strings.Contains(extraClasses, "taken")

		slots = append(slots, Timeslot{
			Time:      timeRange,
			Available: !taken,
		})
	}

	return slots
}

// BookingResult holds the response from a booking attempt.
type BookingResult struct {
	Status   bool   `json:"status"`
	MsgTitle string `json:"msg_title"`
	Msg      string `json:"msg"`
	InsertID int    `json:"insertID"`
}

// BookSlot submits a booking for the given facility, date, and time slot.
func (c *Client) BookSlot(facilityID, unitID, name, contact, date, timeSlot string) (*BookingResult, error) {
	// Build the otherData JSON array matching the browser's format
	otherData := []map[string]string{
		{"name": "fldFacilityId", "value": facilityID},
		{"name": "fldUnitId", "value": unitID},
		{"name": "fldName", "value": name},
		{"name": "fldContact", "value": contact},
		{"name": "fldBookingDate", "value": date},
		{"name": "bookingTime", "value": timeSlot},
		{"name": "_co6sO0rpsfat", "value": c.csrfToken},
	}
	otherDataJSON, err := json.Marshal(otherData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal otherData: %w", err)
	}

	// Build multipart/form-data body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("is_ajax", "1")
	writer.WriteField("otherData", string(otherDataJSON))
	writer.WriteField("_co6sO0rpsfat", c.csrfToken)
	writer.WriteField("fileCount", "0")
	writer.Close()

	req, err := http.NewRequest("POST", c.BaseURL+"/booking/add_new_booking_action", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create booking request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("booking request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read booking response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("booking returned status %d: %s", resp.StatusCode, string(body))
	}

	var result BookingResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse booking response: %w (body: %s)", err, string(body))
	}

	return &result, nil
}

// fetchCSRFToken loads the login page and extracts the CSRF token from the HTML.
func (c *Client) fetchCSRFToken() (string, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/login")
	if err != nil {
		return "", fmt.Errorf("failed to fetch login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login page: %w", err)
	}

	// CSRF token is set via JS: csrf_token = '3798d67d9f62853d5936938c961b5024';
	re := regexp.MustCompile(`csrf_token\s*=\s*'([^']+)'`)
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		// Fallback: try hidden input pattern
		re = regexp.MustCompile(`name="_co6sO0rpsfat"\s+value="([^"]+)"`)
		matches = re.FindSubmatch(body)
	}
	if len(matches) < 2 {
		return "", fmt.Errorf("CSRF token not found in login page HTML")
	}

	return string(matches[1]), nil
}
