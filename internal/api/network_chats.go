// Network chats handler — agent's chat profile (群档案) REST endpoints.
//
// Endpoints (mirror contact endpoints in network.go):
//
//	GET    /api/agents/:id/network/chats              list summaries
//	GET    /api/agents/:id/network/chats/:cid         get full chat
//	PATCH  /api/agents/:id/network/chats/:cid         update title/kind/tags/body
//	DELETE /api/agents/:id/network/chats/:cid         delete
//
// All responses are JSON. Chat IDs accept the same 3 forms as contact IDs
// (canonical "source:externalId", URL-encoded, or filesystem "source-externalId").
package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListChats GET /api/agents/:id/network/chats
func (h *networkHandler) ListChats(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	list, err := s.ListChats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"chats": list})
}

// GetChat GET /api/agents/:id/network/chats/:cid
func (h *networkHandler) GetChat(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))
	chat, err := s.GetChat(cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if chat == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	c.JSON(http.StatusOK, chat)
}

// UpdateChat PATCH /api/agents/:id/network/chats/:cid
// Body: { title?, kind?, tags?, body?, memberCount? }
func (h *networkHandler) UpdateChat(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 512*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var patch struct {
		Title       *string   `json:"title"`
		Kind        *string   `json:"kind"`
		Tags        *[]string `json:"tags"`
		Body        *string   `json:"body"`
		MemberCount *int      `json:"memberCount"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
			return
		}
	}

	chat, err := s.GetChat(cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if chat == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	if patch.Title != nil {
		chat.Title = *patch.Title
	}
	if patch.Kind != nil {
		chat.Kind = *patch.Kind
	}
	if patch.Tags != nil {
		chat.Tags = *patch.Tags
	}
	if patch.Body != nil {
		chat.Body = *patch.Body
	}
	if patch.MemberCount != nil {
		chat.MemberCount = *patch.MemberCount
	}
	if err := s.SaveChat(chat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chat)
}

// DeleteChat DELETE /api/agents/:id/network/chats/:cid
func (h *networkHandler) DeleteChat(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))
	if err := s.DeleteChat(cid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
