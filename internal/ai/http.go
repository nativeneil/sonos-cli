package ai

import (
	"net/http"
	"time"
)

var apiHTTPClient = &http.Client{Timeout: 25 * time.Second}
