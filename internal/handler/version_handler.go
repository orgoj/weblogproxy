package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/version"
)

// VersionHandler returns the current version information
func VersionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":     version.Version,
		"build_date":  version.BuildDate,
		"commit_hash": version.CommitHash,
	})
}
