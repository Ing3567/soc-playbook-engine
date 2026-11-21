package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const authClaimsContextKey = "auth_claims"

var defaultDevClaims = AuthClaims{
	UserID:      "dev_instructor",
	Username:    "instructor",
	DisplayName: "Dev Instructor",
	Role:        "instructor",
}

func extractBearerToken(c *gin.Context) string {
	hdr := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(hdr) > 7 && strings.EqualFold(hdr[:7], "Bearer ") {
		return strings.TrimSpace(hdr[7:])
	}
	return ""
}

// OptionalAuthMiddleware parses a Bearer token if provided without rejecting unauthenticated callers.
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := extractBearerToken(c); token != "" {
			if claims, err := VerifyAuthToken(token, ActiveAuthSecret(), time.Now()); err == nil {
				c.Set(authClaimsContextKey, *claims)
				c.Next()
				return
			}
		}
		if !Config.AuthEnabled {
			c.Set(authClaimsContextKey, defaultDevClaims)
		}
		c.Next()
	}
}

// RequireAuthMiddleware enforces a valid Bearer token when AUTH_ENABLED=true.
// When AUTH_ENABLED=false, valid tokens are still respected; otherwise a default dev instructor principal is attached.
func RequireAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := extractBearerToken(c); token != "" {
			claims, err := VerifyAuthToken(token, ActiveAuthSecret(), time.Now())
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token ไม่ถูกต้องหรือหมดอายุ กรุณาเข้าสู่ระบบใหม่"})
				return
			}
			c.Set(authClaimsContextKey, *claims)
			c.Next()
			return
		}
		if !Config.AuthEnabled {
			c.Set(authClaimsContextKey, defaultDevClaims)
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบก่อนใช้งาน"})
	}
}

func RequireInstructorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := GetAuthClaims(c)
		if !ok {
			if !Config.AuthEnabled {
				c.Set(authClaimsContextKey, defaultDevClaims)
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบก่อนใช้งาน"})
			return
		}
		if Config.AuthEnabled && claims.Role != "instructor" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "สิทธิ์ไม่เพียงพอ (ต้องเป็น Instructor เท่านั้น)"})
			return
		}
		c.Next()
	}
}

func GetAuthClaims(c *gin.Context) (AuthClaims, bool) {
	val, exists := c.Get(authClaimsContextKey)
	if !exists {
		return AuthClaims{}, false
	}
	claims, ok := val.(AuthClaims)
	return claims, ok
}
