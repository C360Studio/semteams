// Package natstest provides disposable, digest-pinned NATS testcontainers brokers for
// SemTeams integration tests (#400), replacing unported natsclient.NewTestClient.
package natstest

import (
	"context"
	_ "embed" // Blank import required for //go:embed nats-image.
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	clientPort           = "4222/tcp"
	readyLog             = "Server is ready"
	maxPayloadConfigPath = "/etc/nats/semteams-natstest.conf"

	startAttempts       = 2
	startBudget         = 90 * time.Second
	clientTimeout       = 15 * time.Second
	dialTimeout         = 5 * time.Second
	clientCloseBudget   = 10 * time.Second
	containerStopBudget = 60 * time.Second
	rollbackBudget      = 15 * time.Second
	maxLogBytes         = 256 << 10

	streamMaxAge   = time.Hour
	streamMaxBytes = 64 << 20
)

//go:embed nats-image
var imagePin string

var digestImage = regexp.MustCompile(`^nats(:[A-Za-z0-9][A-Za-z0-9._-]*)?@sha256:[0-9a-f]{64}$`)

// pinnedImage extracts the single content-addressed NATS image line from raw,
// skipping blank lines and # comments, and refuses tag-only or malformed pins
// before any Docker call is made.
func pinnedImage(raw string) (string, error) {
	var images []string
	for line := range strings.SplitSeq(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			images = append(images, trimmed)
		}
	}
	if len(images) != 1 {
		return "", fmt.Errorf("natstest: nats-image holds %d image lines, want exactly 1", len(images))
	}
	if !digestImage.MatchString(images[0]) {
		return "", fmt.Errorf("natstest: %q is not a valid NATS digest reference (nats[:tag]@sha256:<64-hex>)", images[0])
	}
	return images[0], nil
}

type brokerOptions struct {
	maxPayload    int64
	maxPayloadSet bool
}

func validateOptions(opts []Option) (brokerOptions, error) {
	var cfg brokerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.maxPayloadSet && cfg.maxPayload < 1 {
		return brokerOptions{}, fmt.Errorf("natstest: max payload %d bytes must be >= 1", cfg.maxPayload)
	}
	return cfg, nil
}

// Option configures a Broker started by New.
type Option func(*brokerOptions)

// WithMaxPayload sets the NATS server max_payload limit in bytes. Values below
// 1 are refused before any Docker call.
func WithMaxPayload(bytes int64) Option {
	return func(cfg *brokerOptions) {
		cfg.maxPayload = bytes
		cfg.maxPayloadSet = true
	}
}

// Broker owns one disposable, JetStream-enabled NATS container and connected
// natsclient.Client for an integration test. It retains no context.Context.
type Broker struct {
	Client *natsclient.Client
	URL    string

	mu          sync.Mutex
	container   testcontainers.Container
	containerID string
	stopOnce    sync.Once
	stopErr     error
}

// New starts a JetStream-enabled NATS container from the digest-pinned image,
// verifies JetStream readiness, connects a natsclient.Client, and registers
// bounded t.Cleanup teardown.
func New(t testing.TB, opts ...Option) *Broker {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), startBudget)
	defer cancel()

	b, err := startBroker(ctx, t.Logf, opts...)
	if err != nil {
		t.Fatalf("natstest: start broker: %v", err)
	}

	t.Cleanup(func() {
		if err := b.stop(); err != nil {
			t.Errorf("natstest: cleanup broker: %v", err)
		}
	})
	return b
}

// CreateKVBucket creates (or returns an existing) JetStream KeyValue bucket on
// the test broker.
func (b *Broker) CreateKVBucket(ctx context.Context, name string) (jetstream.KeyValue, error) {
	if ctx == nil {
		return nil, errors.New("natstest: CreateKVBucket with nil context")
	}
	return b.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: name})
}

// CreateStream creates or ensures a bounded JetStream stream on the test broker.
func (b *Broker) CreateStream(ctx context.Context, name string, subjects ...string) (jetstream.Stream, error) {
	if ctx == nil {
		return nil, errors.New("natstest: CreateStream with nil context")
	}
	return b.Client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     name,
		Subjects: subjects,
		MaxAge:   streamMaxAge,
		MaxBytes: streamMaxBytes,
		Discard:  jetstream.DiscardOld,
	})
}

func startBroker(ctx context.Context, logf func(string, ...any), opts ...Option) (*Broker, error) {
	cfg, err := validateOptions(opts)
	if err != nil {
		return nil, err
	}
	image, err := pinnedImage(imagePin)
	if err != nil {
		return nil, err
	}

	prepareDocker(ctx, logf)

	c, err := startContainerWithRetry(ctx, image, cfg, logf)
	if err != nil {
		return nil, err
	}

	url, err := resolveBrokerURL(ctx, c)
	if err != nil {
		rollbackBroker(ctx, nil, c, logf)
		return nil, err
	}

	if err := probeJetStreamReady(ctx, url); err != nil {
		rollbackBroker(ctx, nil, c, logf)
		return nil, err
	}

	client, err := connectNATSClient(ctx, url)
	if err != nil {
		rollbackBroker(ctx, client, c, logf)
		return nil, err
	}

	return &Broker{
		Client:      client,
		URL:         url,
		container:   c,
		containerID: c.GetContainerID(),
	}, nil
}

func startContainerWithRetry(ctx context.Context, image string, cfg brokerOptions, logf func(string, ...any)) (testcontainers.Container, error) {
	cmd := []string{"--port", "4222", "--js"}
	if cfg.maxPayloadSet {
		cmd = append(cmd, "--config", maxPayloadConfigPath)
	}

	var combinedErr error
	for attempt := range startAttempts {
		hostPort, portErr := freePort()
		if portErr != nil {
			return nil, errors.Join(combinedErr, fmt.Errorf("allocate free port: %w", portErr))
		}
		req := testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        image,
				Labels:       map[string]string{testLabel: labelValue()},
				ExposedPorts: []string{clientPort},
				Cmd:          cmd,
				WaitingFor:   wait.ForLog(readyLog),
				HostConfigModifier: func(hc *container.HostConfig) {
					hc.PortBindings = nat.PortMap{
						nat.Port(clientPort): []nat.PortBinding{{HostPort: hostPort}},
					}
				},
			},
			Started: true,
		}
		if cfg.maxPayloadSet {
			req.Files = []testcontainers.ContainerFile{{
				Reader:            strings.NewReader(fmt.Sprintf("max_payload: %d\n", cfg.maxPayload)),
				ContainerFilePath: maxPayloadConfigPath,
				FileMode:          0o644,
			}}
		}

		c, err := testcontainers.GenericContainer(ctx, req)
		if err == nil {
			return c, nil
		}
		if c != nil {
			rollbackBroker(ctx, nil, c, logf)
		}
		combinedErr = errors.Join(combinedErr, err)
		if attempt+1 >= startAttempts || !isAddressInUse(err) {
			return nil, fmt.Errorf("start container: %w", combinedErr)
		}
	}
	return nil, fmt.Errorf("start container: %w", combinedErr)
}

func resolveBrokerURL(ctx context.Context, c testcontainers.Container) (string, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve container host: %w", err)
	}
	mapped, err := c.MappedPort(ctx, nat.Port(clientPort))
	if err != nil {
		return "", fmt.Errorf("resolve mapped port %s: %w", clientPort, err)
	}
	return fmt.Sprintf("nats://%s:%s", host, mapped.Port()), nil
}

func probeJetStreamReady(ctx context.Context, url string) error {
	hostPort := strings.TrimPrefix(url, "nats://")
	dialer := net.Dialer{Timeout: dialTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return fmt.Errorf("dial broker %s: %w", url, err)
	}
	nc, err := nats.Connect(
		url,
		nats.Timeout(dialTimeout),
		nats.MaxReconnects(0),
		nats.SetCustomDialer(preDialedConn{conn: rawConn}),
	)
	if err != nil {
		_ = rawConn.Close()
		return fmt.Errorf("connect probe client: %w", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("create jetstream probe: %w", err)
	}
	if _, err := js.AccountInfo(ctx); err != nil {
		return fmt.Errorf("jetstream readiness check: %w", err)
	}
	return nil
}

type preDialedConn struct {
	conn net.Conn
}

func (d preDialedConn) Dial(_, _ string) (net.Conn, error) {
	return d.conn, nil
}

func connectNATSClient(ctx context.Context, url string) (*natsclient.Client, error) {
	client, err := natsclient.NewClient(
		url,
		natsclient.WithTimeout(clientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, fmt.Errorf("create natsclient: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		return client, fmt.Errorf("connect natsclient: %w", err)
	}
	if err := client.WaitForConnection(connectCtx); err != nil {
		return client, fmt.Errorf("wait for natsclient connection: %w", err)
	}
	return client, nil
}

func (b *Broker) stop() error {
	b.stopOnce.Do(func() {
		b.mu.Lock()
		client := b.Client
		c := b.container
		b.container = nil
		b.mu.Unlock()

		var errs []error
		if client != nil {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), clientCloseBudget)
			if err := client.Close(closeCtx); err != nil {
				errs = append(errs, fmt.Errorf("close client: %w", err))
			}
			closeCancel()
		}
		if c != nil {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), containerStopBudget)
			if err := c.Terminate(stopCtx); err != nil {
				errs = append(errs, fmt.Errorf("terminate container: %w", err))
			}
			stopCancel()
		}
		b.stopErr = errors.Join(errs...)
	})
	return b.stopErr
}

func rollbackBroker(parent context.Context, client *natsclient.Client, c testcontainers.Container, logf func(string, ...any)) {
	baseCtx := context.Background()
	if parent != nil {
		baseCtx = context.WithoutCancel(parent)
	}
	if client != nil {
		closeCtx, closeCancel := context.WithTimeout(baseCtx, clientCloseBudget)
		_ = client.Close(closeCtx)
		closeCancel()
	}
	if c == nil {
		return
	}
	if logf != nil {
		logCtx, logCancel := context.WithTimeout(baseCtx, rollbackBudget)
		if rc, err := c.Logs(logCtx); err == nil {
			data, _ := io.ReadAll(io.LimitReader(rc, maxLogBytes))
			_ = rc.Close()
			if len(data) > 0 {
				logf("natstest: failed startup container %s logs:\n%s", c.GetContainerID(), string(data))
			}
		}
		logCancel()
	}
	termCtx, termCancel := context.WithTimeout(baseCtx, rollbackBudget)
	_ = c.Terminate(termCtx)
	termCancel()
}

func freePort() (string, error) {
	listener, err := net.Listen("tcp4", ":0")
	if err != nil {
		return "", err
	}
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return "", errors.New("unexpected non-TCP listener address")
	}
	port := strconv.Itoa(tcpAddr.Port)
	return port, listener.Close()
}

func isAddressInUse(err error) bool {
	return err != nil && strings.Contains(err.Error(), "address already in use")
}
