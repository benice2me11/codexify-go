package health

import (
	"context"
	"errors"

	"github.com/benice2me11/codexify-go/internal/supervisor"
)

// Composite reports healthy only when every checker passes.
type Composite struct {
	Checkers []supervisor.Checker
}

func (c Composite) Check(ctx context.Context) error {
	var errs []error
	for _, checker := range c.Checkers {
		if err := checker.Check(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
