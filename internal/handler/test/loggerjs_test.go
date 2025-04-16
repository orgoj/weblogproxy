package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/handler"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/stretchr/testify/assert"
)

func TestLoggerJSHandler_MissingSiteID(t *testing.T) {
	// Set up a test Gin context
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a request with no site_id
	req, _ := http.NewRequest("GET", "/logger.js", nil)
	c.Request = req

	// Setup minimal dependencies
	testConfig := &config.Config{}
	testConfig.Server.JavaScript.GlobalObjectName = "wlp"

	ruleProcessor, _ := rules.NewRuleProcessor(testConfig)
	deps := handler.LoggerJSHandlerDeps{
		RuleProcessor:      ruleProcessor,
		Config:             testConfig,
		TokenExpirationDur: 10 * time.Minute,
	}

	// Get the handler
	handlerFunc := handler.NewLoggerJSHandler(deps)

	// Execute the handler
	handlerFunc(c)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK status for missing site_id")
	assert.Contains(t, w.Header().Get("Content-Type"), "application/javascript", "Content-Type should be javascript")
	assert.Contains(t, w.Body.String(), "window.wlp = window.wlp || {}", "Should return basic wlp object")
	assert.Contains(t, w.Body.String(), "window.wlp.log = function() {}", "Should return empty log function")
	assert.NotContains(t, w.Body.String(), "logEnabled: true", "Should not enable logging")
}

func TestLoggerJSHandler_InvalidSiteID(t *testing.T) {
	// Set up a test Gin context
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a request with invalid site_id
	req, _ := http.NewRequest("GET", "/logger.js?site_id=invalid!site", nil)
	c.Request = req

	// Setup minimal dependencies
	testConfig := &config.Config{}
	testConfig.Server.JavaScript.GlobalObjectName = "wlp"

	ruleProcessor, _ := rules.NewRuleProcessor(testConfig)
	deps := handler.LoggerJSHandlerDeps{
		RuleProcessor:      ruleProcessor,
		Config:             testConfig,
		TokenExpirationDur: 10 * time.Minute,
	}

	// Get the handler
	handlerFunc := handler.NewLoggerJSHandler(deps)

	// Execute the handler
	handlerFunc(c)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK status for invalid site_id")
	assert.Contains(t, w.Header().Get("Content-Type"), "application/javascript", "Content-Type should be javascript")
	assert.Contains(t, w.Body.String(), "window.wlp = window.wlp || {}", "Should return basic wlp object")
	assert.Contains(t, w.Body.String(), "window.wlp.log = function() {}", "Should return empty log function")
	assert.NotContains(t, w.Body.String(), "logEnabled: true", "Should not enable logging")
}

func TestLoggerJSHandler_InvalidGtmID(t *testing.T) {
	// Set up a test Gin context
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a request with valid site_id but invalid gtm_id
	req, _ := http.NewRequest("GET", "/logger.js?site_id=valid-site&gtm_id=invalid!gtm", nil)
	c.Request = req

	// Setup minimal dependencies
	testConfig := &config.Config{}
	testConfig.Server.JavaScript.GlobalObjectName = "wlp"

	ruleProcessor, _ := rules.NewRuleProcessor(testConfig)
	deps := handler.LoggerJSHandlerDeps{
		RuleProcessor:      ruleProcessor,
		Config:             testConfig,
		TokenExpirationDur: 10 * time.Minute,
	}

	// Get the handler
	handlerFunc := handler.NewLoggerJSHandler(deps)

	// Execute the handler
	handlerFunc(c)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK status for invalid gtm_id")
	assert.Contains(t, w.Header().Get("Content-Type"), "application/javascript", "Content-Type should be javascript")
	assert.Contains(t, w.Body.String(), "window.wlp = window.wlp || {}", "Should return basic wlp object")
	assert.Contains(t, w.Body.String(), "window.wlp.log = function() {}", "Should return empty log function")
	assert.NotContains(t, w.Body.String(), "logEnabled: true", "Should not enable logging")
}
