package api

import (
	"testing"
	"time"
)

func TestIsDateRangeRejected(t *testing.T) {
	cases := []struct {
		name string
		res  *BookingResult
		want bool
	}{
		{
			name: "date range message",
			res:  &BookingResult{Status: false, Msg: "Date(s) chosen is not within the allowed date range"},
			want: true,
		},
		{
			name: "date range message with title only",
			res:  &BookingResult{Status: false, MsgTitle: "Error", Msg: "Date(s) chosen is not within the allowed date range"},
			want: true,
		},
		{
			name: "slot taken",
			res:  &BookingResult{Status: false, Msg: "The booking slot has been taken."},
			want: false,
		},
		{
			name: "success",
			res:  &BookingResult{Status: true, Msg: "Your booking has been submitted.", InsertID: 123},
			want: false,
		},
		{
			name: "nil",
			res:  nil,
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsDateRangeRejected(c.res); got != c.want {
				t.Fatalf("IsDateRangeRejected(%+v)=%v want %v", c.res, got, c.want)
			}
		})
	}
}

func TestRetryDateRangeRejectionRetriesUntilSuccess(t *testing.T) {
	submits := 0
	sleeps := 0
	fixedNow := time.Now()
	submit := func() (*BookingResult, error) {
		submits++
		if submits < 3 {
			return &BookingResult{Status: false, Msg: "Date(s) chosen is not within the allowed date range"}, nil
		}
		return &BookingResult{Status: true, Msg: "Your booking has been submitted.", InsertID: 999}, nil
	}

	res, err := RetryDateRangeRejection(submit, fixedNow.Add(time.Hour), func() time.Time { return fixedNow }, func(time.Duration) { sleeps++ })

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Status || res.InsertID != 999 {
		t.Fatalf("got result %+v want success with id 999", res)
	}
	if submits != 3 {
		t.Fatalf("submits=%d want 3", submits)
	}
	if sleeps != 2 {
		t.Fatalf("sleeps=%d want 2", sleeps)
	}
}

func TestRetryDateRangeRejectionStopsOnOtherRejection(t *testing.T) {
	submits := 0
	submit := func() (*BookingResult, error) {
		submits++
		return &BookingResult{Status: false, Msg: "The booking slot has been taken."}, nil
	}

	res, err := RetryDateRangeRejection(submit, time.Now().Add(time.Hour), time.Now, func(time.Duration) {})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Msg != "The booking slot has been taken." {
		t.Fatalf("got result %+v want slot-taken rejection", res)
	}
	if submits != 1 {
		t.Fatalf("submits=%d want 1 (no retry on non-date-range rejection)", submits)
	}
}

func TestRetryDateRangeRejectionStopsAtDeadline(t *testing.T) {
	submits := 0
	fixedNow := time.Now()
	submit := func() (*BookingResult, error) {
		submits++
		return &BookingResult{Status: false, Msg: "Date(s) chosen is not within the allowed date range"}, nil
	}

	res, err := RetryDateRangeRejection(submit, fixedNow, func() time.Time { return fixedNow }, func(time.Duration) { t.Fatal("sleep should not run at deadline") })

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Status {
		t.Fatalf("got result %+v want final date-range rejection", res)
	}
	if submits != 1 {
		t.Fatalf("submits=%d want 1 (deadline already reached)", submits)
	}
}

func TestRetryDateRangeRejectionReturnsTransportError(t *testing.T) {
	submits := 0
	submit := func() (*BookingResult, error) {
		submits++
		if submits == 1 {
			return &BookingResult{Status: false, Msg: "Date(s) chosen is not within the allowed date range"}, nil
		}
		return nil, &transportError{msg: "dial tcp: connection timed out"}
	}

	_, err := RetryDateRangeRejection(submit, time.Now().Add(time.Hour), time.Now, func(time.Duration) {})

	if err == nil {
		t.Fatal("want transport error surfaced, got nil")
	}
	if submits != 2 {
		t.Fatalf("submits=%d want 2", submits)
	}
}

type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }
