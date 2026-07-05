package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// CloneRepositoryCommand はリポジトリ clone コマンドの入力 DTO。
type CloneRepositoryCommand struct {
	URL  string
	Dest string
}

// CloneRepositoryResult は clone 結果（clone 先の絶対パス）。
type CloneRepositoryResult struct {
	Path string
}

// CloneRepositoryHandler はリポジトリを clone するハンドラ。
type CloneRepositoryHandler struct {
	git port.GitService
}

// NewCloneRepositoryHandler は依存を注入して CloneRepositoryHandler を返す。
func NewCloneRepositoryHandler(git port.GitService) *CloneRepositoryHandler {
	return &CloneRepositoryHandler{git: git}
}

// Handle は URL を Dest に clone して clone 先パスを返す。
func (h *CloneRepositoryHandler) Handle(_ context.Context, cmd CloneRepositoryCommand) (CloneRepositoryResult, error) {
	url := strings.TrimSpace(cmd.URL)
	if url == "" {
		return CloneRepositoryResult{}, apperr.Validation(fmt.Errorf("clone repository: url is required"))
	}
	dest := strings.TrimSpace(cmd.Dest)
	if dest == "" {
		return CloneRepositoryResult{}, apperr.Validation(fmt.Errorf("clone repository: dest is required"))
	}

	path, err := h.git.Clone(url, dest)
	if err != nil {
		return CloneRepositoryResult{}, apperr.Validation(fmt.Errorf("clone repository: %w", err))
	}
	return CloneRepositoryResult{Path: path}, nil
}
