// Network handler — agent's contact book & progressive-disclosure relations.
//
// Endpoints:
//
//	GET    /api/agents/:id/network/contacts              list summaries
//	GET    /api/agents/:id/network/contacts/:cid         get full contact
//	PATCH  /api/agents/:id/network/contacts/:cid         update body/tags
//	DELETE /api/agents/:id/network/contacts/:cid         delete
//	POST   /api/agents/:id/network/contacts/:cid/merge   body {aliasId}
//	POST   /api/agents/:id/network/refresh               rebuild INDEX.md
//
// All responses are JSON. Contact IDs arrive URL-encoded (":" as %3A) or with
// the filesystem form "source-externalId"; both are accepted via normalization.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/network"
	"github.com/gin-gonic/gin"
)

type networkHandler struct {
	manager *agent.Manager
}

// normalizeContactID accepts either "feishu:ou_abc" (canonical),
// "feishu%3Aou_abc" (URL-encoded), or "feishu-ou_abc" (filesystem form) and
// returns the canonical form.
func normalizeContactID(raw string) string {
	if decoded, err := url.QueryUnescape(raw); err == nil {
		raw = decoded
	}
	if strings.Contains(raw, ":") {
		return raw
	}
	// Filesystem form: first "-" separates source from externalId.
	if i := strings.Index(raw, "-"); i > 0 {
		return raw[:i] + ":" + raw[i+1:]
	}
	return raw
}

func (h *networkHandler) storeFor(c *gin.Context) (*network.Store, bool) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return nil, false
	}
	return network.NewStore(ag.WorkspaceDir), true
}

// ListContacts GET /api/agents/:id/network/contacts
func (h *networkHandler) ListContacts(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	list, err := s.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contacts": list})
}

// GetContact GET /api/agents/:id/network/contacts/:cid
func (h *networkHandler) GetContact(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))
	contact, err := s.Get(cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if contact == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}
	c.JSON(http.StatusOK, contact)
}

// UpdateContact PATCH /api/agents/:id/network/contacts/:cid
// Body: { displayName?, tags?, body?, isOwner? }
func (h *networkHandler) UpdateContact(c *gin.Context) {
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
		DisplayName *string   `json:"displayName"`
		Tags        *[]string `json:"tags"`
		Body        *string   `json:"body"`
		IsOwner     *bool     `json:"isOwner"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
			return
		}
	}

	contact, err := s.Get(cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if contact == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}
	if patch.DisplayName != nil {
		contact.DisplayName = *patch.DisplayName
	}
	if patch.Tags != nil {
		contact.Tags = *patch.Tags
	}
	if patch.Body != nil {
		contact.Body = *patch.Body
	}
	if patch.IsOwner != nil {
		contact.IsOwner = *patch.IsOwner
	}
	if err := s.Save(contact); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, contact)
}

// DeleteContact DELETE /api/agents/:id/network/contacts/:cid
func (h *networkHandler) DeleteContact(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))
	if err := s.Delete(cid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MergeContact POST /api/agents/:id/network/contacts/:cid/merge  body {aliasId}
// Marks aliasId as an alias of :cid (moves aliasId into cid.Aliases, bumps
// msgCount on primary). The alias contact file is deleted.
func (h *networkHandler) MergeContact(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	cid := normalizeContactID(c.Param("cid"))
	var body struct {
		AliasID string `json:"aliasId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must be {aliasId: ...}"})
		return
	}
	aliasID := normalizeContactID(body.AliasID)
	if aliasID == "" || aliasID == cid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "aliasId required and must differ from cid"})
		return
	}

	primary, err := s.Get(cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if primary == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "primary contact not found"})
		return
	}
	alias, err := s.Get(aliasID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if alias == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "alias contact not found"})
		return
	}
	// Move alias ID into primary.Aliases (dedupe).
	if !containsStr(primary.Aliases, aliasID) {
		primary.Aliases = append(primary.Aliases, aliasID)
	}
	// Merge msgCount and LastSeenAt (keep latest).
	primary.MsgCount += alias.MsgCount
	if alias.LastSeenAt.After(primary.LastSeenAt) {
		primary.LastSeenAt = alias.LastSeenAt
	}
	// Absorb alias tags (dedupe).
	for _, t := range alias.Tags {
		if !containsStr(primary.Tags, t) {
			primary.Tags = append(primary.Tags, t)
		}
	}
	if err := s.Save(primary); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.Delete(aliasID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, primary)
}

// RefreshIndex POST /api/agents/:id/network/refresh
func (h *networkHandler) RefreshIndex(c *gin.Context) {
	s, ok := h.storeFor(c)
	if !ok {
		return
	}
	if err := s.RefreshIndex(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func containsStr(arr []string, v string) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}
