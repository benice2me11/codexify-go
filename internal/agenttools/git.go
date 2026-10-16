package agenttools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

type Git struct {
	Root *workspace.Root
}

type GitStatusInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
}

type GitOutput struct {
	Repository *RepositoryContext `json:"repository,omitempty"`
	Output     string             `json:"output"`
}

func (g *Git) Status(context.Context, GitStatusInput) (GitOutput, error) {
	return g.run("status", "--short", "--branch")
}

type GitDiffInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
	Staged     bool   `json:"staged,omitempty" jsonschema:"show staged diff instead of working tree diff"`
}

func (g *Git) Diff(_ context.Context, in GitDiffInput) (GitOutput, error) {
	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if in.Staged {
		args = append(args, "--cached")
	}
	return g.run(args...)
}

type GitLogInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
	Count      int    `json:"count,omitempty" jsonschema:"number of commits, default 10 and maximum 100"`
}

func (g *Git) Log(_ context.Context, in GitLogInput) (GitOutput, error) {
	count := in.Count
	if count <= 0 {
		count = 10
	}
	if count > 100 {
		count = 100
	}
	return g.run("log", "--no-color", "--date=iso-strict", "--pretty=format:%h%x09%ad%x09%an%x09%s", "-n", strconv.Itoa(count))
}

func (g *Git) run(args ...string) (GitOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	all := append([]string{"-C", g.Root.Path()}, args...)
	cmd := exec.CommandContext(ctx, "git", all...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return GitOutput{}, errors.New("git command timed out")
	}
	text := string(out)
	if len(text) > 2<<20 {
		text = text[:2<<20] + "\n[output truncated]\n"
	}
	if err != nil {
		return GitOutput{}, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(text))
	}
	return GitOutput{Output: text}, nil
}
