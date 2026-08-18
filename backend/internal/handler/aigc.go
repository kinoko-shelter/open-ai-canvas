package handler

import (
	"net/http"
	"strconv"

	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterAigcRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.GET("/aigc/departments", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		departments, err := svc.AigcDepartments(user, c.Query("keyword"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"departments": departments})
	})
	r.POST("/aigc/departments", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AigcDepartmentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		department, err := svc.CreateAigcDepartment(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"department": department})
	})
	r.PATCH("/aigc/departments/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AigcDepartmentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		department, err := svc.UpdateAigcDepartment(user, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"department": department})
	})
	r.GET("/aigc/projects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		projects, err := svc.AigcProjects(user, service.AdminListQuery{Keyword: c.Query("keyword"), Status: c.Query("status"), Page: page, Limit: limit}, c.Query("level"), c.Query("parentId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, projects)
	})
	r.GET("/aigc/projects/available", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		projects, err := svc.AvailableAigcSecondLevelProjects(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"projects": projects})
	})
	r.POST("/aigc/projects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AigcProjectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		project, err := svc.CreateAigcProject(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"project": project})
	})
	r.PATCH("/aigc/projects/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AigcProjectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		project, err := svc.UpdateAigcProject(user, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"project": project})
	})
	r.DELETE("/aigc/projects/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.DeleteAigcProject(user, c.Param("id")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})
}
