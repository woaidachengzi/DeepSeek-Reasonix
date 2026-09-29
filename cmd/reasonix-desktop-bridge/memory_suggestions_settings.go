package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/memory"
	"reasonix/internal/memorysuggest"
	"reasonix/internal/pathidentity"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/skill"
)

const previewMemorySuggestionSessionLimit = 12

type previewMemorySuggestionAcceptance struct {
	WorkspaceRoot string `json:"workspaceRoot"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
}

type previewMemorySuggestionAcceptanceView struct {
	Path        string                              `json:"path"`
	Suggestions memorysuggest.MemorySuggestionsView `json:"suggestions"`
}

func previewMemorySuggestionSkills(root string) (*skill.Store, []skill.Skill, error) {
	cfg, err := appconfig.LoadForRootWithoutCredentialsReadOnly(root)
	if err != nil {
		return nil, nil, err
	}
	opts := skill.Options{
		ProjectRoot: root, CustomPaths: cfg.SkillCustomPaths(),
		PluginPaths: cfg.PluginPackageSkillOwners(), PluginAgentPaths: cfg.PluginPackageAgentOwners(),
		MaxDepth: cfg.SkillMaxDepth(), Stderr: io.Discard,
	}
	opts.ExcludedPaths = cfg.SkillExcludedPaths()
	store := skill.New(opts)
	return store, store.List(), nil
}

func loadPreviewMemorySuggestions(workspaceRoot string) (memorysuggest.MemorySuggestionsView, *memory.Set, string, *skill.Store, error) {
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil {
		return memorysuggest.MemorySuggestionsView{}, nil, "", nil, err
	}
	if root == "" {
		return memorysuggest.MemorySuggestionsView{}, nil, "", nil, fmt.Errorf("a workspace is required for memory suggestions")
	}
	set, _, err := loadMemorySetForPreview(root)
	if err != nil {
		return memorysuggest.MemorySuggestionsView{}, nil, "", nil, err
	}
	store, existingSkills, err := previewMemorySuggestionSkills(root)
	if err != nil {
		return memorysuggest.MemorySuggestionsView{}, nil, "", nil, err
	}

	allowed := map[string]struct{}{}
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath != "" {
		identities, _, openErr := sessionidentity.OpenReadOnlyIfExists(context.Background(), identityPath, appconfig.SessionProfileRoot())
		if openErr != nil {
			return memorysuggest.MemorySuggestionsView{}, nil, "", nil, openErr
		}
		if identities != nil {
			defer identities.Close()
			records, listErr := identities.List(context.Background())
			if listErr != nil {
				return memorysuggest.MemorySuggestionsView{}, nil, "", nil, listErr
			}
			workspaceKey := pathidentity.Canonical(root)
			for _, record := range records {
				if record.State == sessionidentity.StateReady && pathidentity.Canonical(record.WorkspaceRoot) == workspaceKey {
					allowed[record.ID] = struct{}{}
				}
			}
		}
	}
	sessions := memorysuggest.LoadSessions(appconfig.SessionDir(), previewMemorySuggestionSessionLimit, allowed)
	view := memorysuggest.Generate(set, root, existingSkills, sessions)
	return view, set, root, store, nil
}

func (b *bridgeServer) memorySuggestions(w http.ResponseWriter, r *http.Request) {
	view, _, _, _, err := loadPreviewMemorySuggestions(r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview memory suggestions")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) acceptMemorySuggestion(w http.ResponseWriter, r *http.Request) {
	var request previewMemorySuggestionAcceptance
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid memory suggestion acceptance")
		return
	}
	if strings.TrimSpace(request.ID) == "" || len(request.ID) > 256 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "memory suggestion identity is required")
		return
	}
	view, set, _, skillStore, err := loadPreviewMemorySuggestions(request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview memory suggestions")
		return
	}
	path := ""
	switch request.Kind {
	case "memory":
		for _, candidate := range view.Memories {
			if candidate.ID != request.ID {
				continue
			}
			result, saveErr := set.Store.SaveWithOptions(memory.Memory{
				Name: candidate.Name, Title: candidate.Title, Description: candidate.Description,
				Type: memory.NormalizeType(candidate.Type), Scope: memory.NormalizeFactScope(candidate.Scope), Body: candidate.Body,
			}, memory.SaveOptions{RequireCreate: true})
			if saveErr != nil {
				writeProtocolError(w, http.StatusConflict, "conflict", "memory suggestion could not be saved; refresh and review it again")
				return
			}
			path = result.Path
			break
		}
	case "skill":
		for _, candidate := range view.Skills {
			if candidate.ID != request.ID {
				continue
			}
			scope := skill.ScopeProject
			if candidate.Scope == "global" || !skillStore.HasProjectScope() {
				scope = skill.ScopeGlobal
			}
			content := skill.RenderSkillFile(skill.SkillFileOptions{Name: candidate.Name, Description: candidate.Description, Body: candidate.Body})
			created, createErr := skillStore.CreateWithContent(candidate.Name, scope, content)
			if createErr != nil {
				writeProtocolError(w, http.StatusConflict, "conflict", "skill suggestion could not be saved; refresh and review it again")
				return
			}
			path = created
			break
		}
	default:
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid memory suggestion kind")
		return
	}
	if path == "" {
		writeProtocolError(w, http.StatusConflict, "conflict", "suggestion is stale or no longer available; refresh and review it again")
		return
	}
	next, _, _, _, err := loadPreviewMemorySuggestions(request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved suggestion but failed to refresh suggestions")
		return
	}
	writeJSON(w, http.StatusOK, previewMemorySuggestionAcceptanceView{Path: path, Suggestions: next})
}
