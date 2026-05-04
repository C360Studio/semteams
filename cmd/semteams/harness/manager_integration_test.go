//go:build integration

package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ManagerIntegrationSuite struct {
	suite.Suite
	testClient *natsclient.TestClient
	natsClient *natsclient.Client
	manager    *Manager
	ctx        context.Context
	cancel     context.CancelFunc
}

func (s *ManagerIntegrationSuite) SetupSuite() {
	s.testClient = natsclient.NewTestClient(s.T(),
		natsclient.WithJetStream(),
		natsclient.WithKV())
	s.natsClient = s.testClient.Client
}

func (s *ManagerIntegrationSuite) SetupTest() {
	var err error
	s.manager, err = NewManager(s.natsClient)
	s.Require().NoError(err)
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)
}

func (s *ManagerIntegrationSuite) TearDownTest() {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if keys, err := s.manager.kv.Keys(cleanupCtx); err == nil {
		for _, k := range keys {
			_ = s.manager.kv.Delete(cleanupCtx, k)
		}
	}
	s.cancel()
}

func TestManagerIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ManagerIntegrationSuite))
}

func (s *ManagerIntegrationSuite) TestPutGet() {
	h := &Harness{
		Name:                "stub",
		ComposeProfile:      "harness-stub",
		Image:               "scratch",
		SmokeContractSchema: "stub.smoke_contract.v1",
		DomainDescription:   "stub for testing",
	}
	require.NoError(s.T(), s.manager.Put(s.ctx, h))

	got, err := s.manager.Get(s.ctx, "stub")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "stub", got.Name)
	require.Equal(s.T(), "harness-stub", got.ComposeProfile)
}

func (s *ManagerIntegrationSuite) TestGetMissing() {
	_, err := s.manager.Get(s.ctx, "ghost")
	require.Error(s.T(), err)
}

func (s *ManagerIntegrationSuite) TestPutValidationFails() {
	bad := &Harness{Name: ""} // missing required fields
	err := s.manager.Put(s.ctx, bad)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "validate")
}

func (s *ManagerIntegrationSuite) TestListEmpty() {
	out, err := s.manager.List(s.ctx)
	require.NoError(s.T(), err)
	require.Empty(s.T(), out)
}

func (s *ManagerIntegrationSuite) TestListSorted() {
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		require.NoError(s.T(), s.manager.Put(s.ctx, &Harness{
			Name:                name,
			ComposeProfile:      "p",
			Image:               "i",
			SmokeContractSchema: "s.v1",
			DomainDescription:   "d",
		}))
	}
	out, err := s.manager.List(s.ctx)
	require.NoError(s.T(), err)
	require.Len(s.T(), out, 3)
	require.Equal(s.T(), "alpha", out[0].Name)
	require.Equal(s.T(), "bravo", out[1].Name)
	require.Equal(s.T(), "charlie", out[2].Name)
}

func (s *ManagerIntegrationSuite) TestLoadFromFileMissing() {
	n, err := s.manager.LoadFromFile(s.ctx, "/nonexistent/path.json")
	require.NoError(s.T(), err) // missing file is not an error
	require.Equal(s.T(), 0, n)
}

func (s *ManagerIntegrationSuite) TestLoadFromFile() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "harnesses.json")
	body := `{"harnesses":[
		{"name":"alpha","compose_profile":"p","image":"i","smoke_contract_schema":"s.v1","domain_description":"d"},
		{"name":"bravo","compose_profile":"p","image":"i","smoke_contract_schema":"s.v1","domain_description":"d"}
	]}`
	require.NoError(s.T(), os.WriteFile(path, []byte(body), 0o644))

	n, err := s.manager.LoadFromFile(s.ctx, path)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, n)

	out, err := s.manager.List(s.ctx)
	require.NoError(s.T(), err)
	require.Len(s.T(), out, 2)
}

func (s *ManagerIntegrationSuite) TestLoadFromFileMalformed() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "harnesses.json")
	require.NoError(s.T(), os.WriteFile(path, []byte(`{not json`), 0o644))

	_, err := s.manager.LoadFromFile(s.ctx, path)
	require.Error(s.T(), err)
}

func (s *ManagerIntegrationSuite) TestLoadFromFileIdempotent() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "harnesses.json")
	body := `{"harnesses":[{"name":"alpha","compose_profile":"p","image":"i","smoke_contract_schema":"s.v1","domain_description":"d"}]}`
	require.NoError(s.T(), os.WriteFile(path, []byte(body), 0o644))

	for i := 0; i < 3; i++ {
		n, err := s.manager.LoadFromFile(s.ctx, path)
		require.NoError(s.T(), err)
		require.Equal(s.T(), 1, n)
	}
	out, err := s.manager.List(s.ctx)
	require.NoError(s.T(), err)
	require.Len(s.T(), out, 1)
}
