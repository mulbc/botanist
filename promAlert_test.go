package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func Test_promAlertHandler(t *testing.T) {
	// Test GET requests - should not work
	req, err := http.NewRequest("GET", "/alert", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := sendRequest(req)
	assertEqual(t, rr.Code, http.StatusMethodNotAllowed, "")

	// Test to Post a valid alert
	testFile, err := os.Open("promTest.json")
	if err != nil {
		t.Fatal(err)
	}
	req, err = http.NewRequest("POST", "/alert", testFile)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rr = sendRequest(req)
	assertEqual(t, rr.Code, http.StatusOK, "")

	req, err = http.NewRequest("POST", "/alert", strings.NewReader("{\"abc\": \"\"}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rr = sendRequest(req)
	assertEqual(t, rr.Code, http.StatusOK, "")
}

func assertEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a == b {
		return
	}
	if len(message) == 0 {
		message = fmt.Sprintf("%v != %v", a, b)
	}
	t.Fatal(message)
}

func sendRequest(request *http.Request) *httptest.ResponseRecorder {
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	response := httptest.NewRecorder()
	handler := http.HandlerFunc(promAlertHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(response, request)
	return response
}
