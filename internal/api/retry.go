package api

import (
	"strings"
	"time"
)

const dateRangeRejectMsg = "Date(s) chosen is not within the allowed date range"

// RetryDelay is the pause between date-range retries.
const RetryDelay = time.Second

// IsDateRangeRejected reports whether a booking result is gpropsystems' "chosen
// date not within the allowed date range" rejection. Facilities allow booking up
// to today + facilityMaxDateRange (7) days; the +7 target rolls into the window
// just after midnight, so a single fire at 00:00:00.000 can land a moment early.
func IsDateRangeRejected(r *BookingResult) bool {
	return r != nil && !r.Status && strings.Contains(r.Msg, dateRangeRejectMsg)
}

// RetryDateRangeRejection re-submits while the booking keeps being rejected as
// out-of-range and now is still before deadline. It stops on success, on any
// non-date-range outcome, on transport error, or once the deadline passes —
// returning the final result so the caller reports it as it would a normal fire.
func RetryDateRangeRejection(submit func() (*BookingResult, error), deadline time.Time, now func() time.Time, sleep func(time.Duration)) (*BookingResult, error) {
	for {
		res, err := submit()
		if err != nil {
			return res, err
		}
		if !IsDateRangeRejected(res) || !now().Before(deadline) {
			return res, nil
		}
		sleep(RetryDelay)
	}
}
