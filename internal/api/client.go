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

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		DisableCompression: true, // Disable automatic decompression, we'll handle it manually
	}

	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: transport,
			Jar:       jar,
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
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", c.BaseURL)
	req.Header.Set("Referer", c.BaseURL+"/landing/parkcity-club-house")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0")

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
	// Strip UTF-8 BOM if present (server sometimes adds it)
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		body = body[3:]
	}

	var loginResp map[string]interface{}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return fmt.Errorf("failed to parse login response (got non-JSON, status %d): %s", resp.StatusCode, preview)
	}

	// Check for login success - API uses "msg" field with success message
	// Note: "stat" field is unreliable (returns false even on success)
	msg, _ := loginResp["msg"].(string)
	if msg == "" {
		// Fallback: check legacy "status" field
		if status, ok := loginResp["status"].(bool); ok && !status {
			msgTitle, _ := loginResp["msg_title"].(string)
			return fmt.Errorf("login failed: %s", msgTitle)
		}
	} else if msg != "Logged In Successfully" {
		return fmt.Errorf("login failed: %s", msg)
	}

	// Update CSRF token from response if present (server may rotate it)
	if newToken, ok := loginResp["_co6sO0rpsfat"].(string); ok && newToken != "" {
		c.csrfToken = newToken
	}

	fmt.Printf("  Login status: %v\n", msg)
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

	// Strip UTF-8 BOM if present (server sometimes adds it)
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		body = body[3:]
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

	// Strip UTF-8 BOM if present (server sometimes adds it)
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		body = body[3:]
	}

	var result BookingResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse booking response: %w (body: %s)", err, string(body))
	}

	return &result, nil
}

// Facility represents a court/facility with its ID and name.
type Facility struct {
	ID   string // e.g. "7935"
	Name string // e.g. "Pickleball Court P1"
}

// UserProfile represents user profile information from the API.
type UserProfile struct {
	Name    string `json:"fldName"`
	Contact string `json:"fldContact"`
}

// UserAPIResponse represents the API response structure.
type UserAPIResponse struct {
	Status  bool        `json:"status"`
	Results UserProfile `json:"results"` // Single object for get_user_info
}

// UserUnitAPIResponse represents the API response structure for get_unit_user.
type UserUnitAPIResponse struct {
	Status  bool          `json:"status"`
	Results []UserProfile `json:"results"` // Array for get_unit_user
}

// Booking represents a single booking entry from the listing.
type Booking struct {
	ID        string // e.g. "399637"
	BookingNo string // e.g. "BK399637"
	Facility  string // e.g. "Pickleball Court P3"
	Date      string // e.g. "2026-04-24"
	TimeStart string // e.g. "07:00:00"
	TimeEnd   string // e.g. "09:00:00"
	Status    string // e.g. "Approved"
}

// GetBookings fetches the list of bookings from today onwards.
func (c *Client) GetBookings() ([]Booking, error) {
	today := time.Now().Format("2006-01-02")
	endpoint := fmt.Sprintf("%s/booking/booking_listing?sEcho=1&iColumns=7&iDisplayStart=0&iDisplayLength=50&mDataProp_0=fldBookingNo&mDataProp_1=fldFName&mDataProp_2=fldUnitNumber&mDataProp_3=bookTime&mDataProp_4=fldAmountPayable&mDataProp_5=fldApproval&mDataProp_6=actions&filterDateFrom=%s", c.BaseURL, today)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create bookings request: %w", err)
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bookings request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bookings response: %w", err)
	}

	// Strip UTF-8 BOM if present
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		body = body[3:]
	}

	var listResp struct {
		Data []struct {
			BookingID string `json:"fldBookingId"`
			BookingNo string `json:"fldBookingNo"`
			Facility  string `json:"fldFName"`
			Date      string `json:"fldBookingDate"`
			TimeStart string `json:"fldBookingTimeStart"`
			TimeEnd   string `json:"fldBookingTimeEnd"`
			Approval  string `json:"fldApproval"`
		} `json:"aaData"`
	}

	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse bookings response: %w", err)
	}

	var bookings []Booking
	for _, b := range listResp.Data {
		// Extract status from HTML like <span class="badge bg-success-400">Approved</span>
		status := "Unknown"
		if strings.Contains(b.Approval, "Approved") {
			status = "Approved"
		} else if strings.Contains(b.Approval, "Pending") {
			status = "Pending"
		} else if strings.Contains(b.Approval, "Rejected") {
			status = "Rejected"
		} else if strings.Contains(b.Approval, "Cancelled") {
			status = "Cancelled"
		}

		bookings = append(bookings, Booking{
			ID:        b.BookingID,
			BookingNo: b.BookingNo,
			Facility:  b.Facility,
			Date:      b.Date,
			TimeStart: b.TimeStart,
			TimeEnd:   b.TimeEnd,
			Status:    status,
		})
	}

	return bookings, nil
}

// GetFacilities fetches the list of available facilities/courts from the booking page.
func (c *Client) GetFacilities() ([]Facility, error) {
	endpoint := c.BaseURL + "/booking/add_new_booking"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create facilities request: %w", err)
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("facilities request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read facilities response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilities request returned status %d", resp.StatusCode)
	}

	return parseFacilityHTML(string(body)), nil
}

// GetUserProfile fetches user profile information (name, contact) from the API.
func (c *Client) GetUserProfile() (*UserProfile, error) {
	endpoint := c.BaseURL + "/booking/get_user_info"

	form := url.Values{}
	form.Set("is_ajax", "1")
	form.Set("_co6sO0rpsfat", c.csrfToken)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create user profile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", c.BaseURL)
	req.Header.Set("Referer", c.BaseURL+"/booking/add_new_booking")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user profile response: %w", err)
	}

	var response UserAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse user profile response: %w", err)
	}

	if !response.Status {
		return nil, fmt.Errorf("API returned status false")
	}

	return &response.Results, nil
}

// GetUnitUserProfile fetches unit-specific user profile information from the API.
func (c *Client) GetUnitUserProfile(unitID string) (*UserProfile, error) {
	endpoint := c.BaseURL + "/booking/get_unit_user"

	form := url.Values{}
	form.Set("is_ajax", "1")
	form.Set("unitId", unitID)
	form.Set("_co6sO0rpsfat", c.csrfToken)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create unit user profile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", c.BaseURL)
	req.Header.Set("Referer", c.BaseURL+"/booking/add_new_booking")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unit user profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read unit user profile response: %w", err)
	}

	var response UserUnitAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse unit user profile response: %w", err)
	}

	if !response.Status || len(response.Results) == 0 {
		return nil, fmt.Errorf("API returned no results")
	}

	return &response.Results[0], nil
}

// ResolveCourtNameToID resolves a court name (e.g., "Pickleball Court P1") to its ID (e.g., "7935").
// If the input is already a numeric ID, it returns it as-is.
// Returns an error if the name cannot be resolved.
func (c *Client) ResolveCourtNameToID(courtInput string) (string, error) {
	// If it's already a numeric ID, return it as-is
	if isNumeric(courtInput) {
		return courtInput, nil
	}

	// Fetch facilities to build a name-to-ID mapping
	facilities, err := c.GetFacilities()
	if err != nil {
		return "", fmt.Errorf("failed to fetch facilities for name resolution: %w", err)
	}

	// Build case-insensitive name-to-ID map
	nameToID := make(map[string]string)
	for _, f := range facilities {
		lowerName := strings.ToLower(f.Name)
		nameToID[lowerName] = f.ID
	}

	// Try to match the input (case-insensitive)
	lowerInput := strings.ToLower(strings.TrimSpace(courtInput))
	if id, ok := nameToID[lowerInput]; ok {
		return id, nil
	}

	// Try partial match (e.g., "p1" matches "Pickleball Court P1")
	for name, id := range nameToID {
		if strings.Contains(name, lowerInput) || strings.Contains(lowerInput, name) {
			return id, nil
		}
	}

	// List available names for error message
	var names []string
	for _, f := range facilities {
		names = append(names, f.Name)
	}
	return "", fmt.Errorf("court name %q not found. Available courts: %v", courtInput, names)
}

// isNumeric checks if a string contains only digits.
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// parseFacilityHTML extracts facility IDs and names from the booking page HTML.
// Matches <option value="ID">Name</option> patterns in the facility dropdown.
func parseFacilityHTML(html string) []Facility {
	var facilities []Facility

	// Match option tags: <option value="ID">Name</option>
	re := regexp.MustCompile(`<option\s+value="(\d+)"[^>]*>([^<]+)</option>`)
	matches := re.FindAllStringSubmatch(html, -1)

	for _, m := range matches {
		id := m[1]
		name := strings.TrimSpace(m[2])
		if name != "" && name != "Select Facility" {
			facilities = append(facilities, Facility{
				ID:   id,
				Name: name,
			})
		}
	}

	return facilities
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
