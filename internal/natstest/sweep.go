package natstest

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/testcontainers/testcontainers-go"
)

const (
	// testLabel marks every NATS container started by internal/natstest so stale
	// containers left by killed test runs are swept narrowly without touching
	// any other repository's containers.
	testLabel = "org.c360studio.semteams.test"
	// sweepAge is three times go test's default 10-minute timeout, matching
	// SemEngine's natsfixture sweepAge.
	sweepAge    = 30 * time.Minute
	sweepBudget = 15 * time.Second
)

var dockerOnce sync.Once

func prepareDocker(ctx context.Context, logf func(format string, args ...any)) {
	dockerOnce.Do(func() {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil && logf != nil {
			logf("natstest: set TESTCONTAINERS_RYUK_DISABLED: %v", err)
		}
		sweepCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sweepBudget)
		defer cancel()
		sweepStaleContainers(sweepCtx, testLabel, sweepAge, logf)
	})
}

func labelValue() string {
	pkg := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok && info.Path != "" {
		pkg = strings.TrimSuffix(info.Path, ".test")
	}
	return fmt.Sprintf("%s pid %d", pkg, os.Getpid())
}

func sweepStaleContainers(ctx context.Context, label string, maxAge time.Duration, logf func(format string, args ...any)) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		if logf != nil {
			logf("natstest: sweep: docker client: %v", err)
		}
		return
	}
	defer func() { _ = cli.Close() }()

	items, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", label)),
	})
	if err != nil {
		if logf != nil {
			logf("natstest: sweep: list containers labelled %s: %v", label, err)
		}
		return
	}

	for _, item := range items {
		if time.Since(time.Unix(item.Created, 0)) > maxAge {
			err := cli.ContainerRemove(ctx, item.ID, container.RemoveOptions{Force: true})
			if err != nil && !cerrdefs.IsNotFound(err) && logf != nil {
				logf("natstest: sweep: remove container %s: %v", item.ID, err)
			}
		}
	}
}
