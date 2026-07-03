package enrich

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"github.com/b42labs/northwatch/internal/config"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/projects"
)

// OpenStackProvider enriches OVN entities with OpenStack metadata.
// Function fields allow testing without real API calls.
type OpenStackProvider struct {
	LookupServer  func(ctx context.Context, serverID string) (name string, err error)
	LookupProject func(ctx context.Context, projectID string) (name string, err error)
}

// NewOpenStackProvider creates an OpenStackProvider authenticated against Keystone.
func NewOpenStackProvider(ctx context.Context, cfg *config.Config) (*OpenStackProvider, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.OpenStackAuthURL,
		Username:         cfg.OpenStackUsername,
		Password:         cfg.OpenStackPassword,
		DomainName:       cfg.OpenStackDomainName,
		TenantName:       cfg.OpenStackProjectName,
	}

	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, fmt.Errorf("creating OpenStack client: %w", err)
	}

	// Trust a custom CA bundle (e.g. a private testbed CA referenced as
	// clouds.yaml `cacert`) when one is configured. This scopes the trust to the
	// OpenStack API client rather than the whole process. AuthenticatedClient
	// uses the default HTTP client, so we authenticate explicitly here instead.
	if cfg.OpenStackCACert != "" {
		httpClient, err := caCertHTTPClient(cfg.OpenStackCACert)
		if err != nil {
			return nil, err
		}
		provider.HTTPClient = *httpClient
	}

	if err := openstack.Authenticate(ctx, provider, opts); err != nil {
		return nil, fmt.Errorf("authenticating with OpenStack: %w", err)
	}

	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: cfg.OpenStackRegionName,
	})
	if err != nil {
		return nil, fmt.Errorf("creating compute client: %w", err)
	}

	identityClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{
		Region: cfg.OpenStackRegionName,
	})
	if err != nil {
		return nil, fmt.Errorf("creating identity client: %w", err)
	}

	return &OpenStackProvider{
		LookupServer: func(ctx context.Context, serverID string) (string, error) {
			srv, err := servers.Get(ctx, computeClient, serverID).Extract()
			if err != nil {
				return "", err
			}
			return srv.Name, nil
		},
		LookupProject: func(ctx context.Context, projectID string) (string, error) {
			proj, err := projects.Get(ctx, identityClient, projectID).Extract()
			if err != nil {
				return "", err
			}
			return proj.Name, nil
		},
	}, nil
}

// caCertHTTPClient builds an http.Client whose TLS root pool is the system
// pool plus the PEM bundle at caCertPath. Used to verify OpenStack APIs fronted
// by a private CA (e.g. an OSISM testbed's testbed.pem).
func caCertHTTPClient(caCertPath string) (*http.Client, error) {
	pem, err := os.ReadFile(caCertPath) // #nosec G304 -- operator-supplied CA cert path (flag/env), not attacker input
	if err != nil {
		return nil, fmt.Errorf("reading OpenStack CA cert %q: %w", caCertPath, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no PEM certificates found in OpenStack CA cert %q", caCertPath)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

func (p *OpenStackProvider) EnrichPort(ctx context.Context, externalIDs map[string]string) (*Info, error) {
	info := &Info{}

	// Extract from external_ids first (no API call needed)
	info.DisplayName = externalIDs["neutron:port_name"]
	info.DeviceOwner = externalIDs["neutron:device_owner"]
	info.DeviceID = externalIDs["neutron:device_id"]
	info.ProjectID = externalIDs["neutron:project_id"]

	// Resolve device_id → server name via Nova
	if info.DeviceID != "" && info.DeviceOwner == "compute:nova" && p.LookupServer != nil {
		if name, err := p.LookupServer(ctx, info.DeviceID); err == nil {
			info.DeviceName = name
		}
	}

	// Resolve project_id → project name via Keystone
	if info.ProjectID != "" && p.LookupProject != nil {
		if name, err := p.LookupProject(ctx, info.ProjectID); err == nil {
			info.ProjectName = name
		}
	}

	if info.DisplayName == "" && info.DeviceOwner == "" && info.ProjectID == "" {
		return nil, nil
	}

	return info, nil
}

func (p *OpenStackProvider) EnrichNetwork(ctx context.Context, externalIDs map[string]string) (*Info, error) {
	info := &Info{}

	info.DisplayName = externalIDs["neutron:network_name"]
	info.ProjectID = externalIDs["neutron:project_id"]

	if info.ProjectID != "" && p.LookupProject != nil {
		if name, err := p.LookupProject(ctx, info.ProjectID); err == nil {
			info.ProjectName = name
		}
	}

	if info.DisplayName == "" && info.ProjectID == "" {
		return nil, nil
	}

	return info, nil
}

func (p *OpenStackProvider) EnrichRouter(ctx context.Context, externalIDs map[string]string) (*Info, error) {
	info := &Info{}

	info.DisplayName = externalIDs["neutron:router_name"]
	info.ProjectID = externalIDs["neutron:project_id"]

	if info.ProjectID != "" && p.LookupProject != nil {
		if name, err := p.LookupProject(ctx, info.ProjectID); err == nil {
			info.ProjectName = name
		}
	}

	if info.DisplayName == "" && info.ProjectID == "" {
		return nil, nil
	}

	return info, nil
}

func (p *OpenStackProvider) EnrichNAT(ctx context.Context, externalIDs map[string]string) (*Info, error) {
	extra := make(map[string]string)

	if fipID := externalIDs["neutron:fip_id"]; fipID != "" {
		extra["fip_id"] = fipID
	}
	if fipMAC := externalIDs["neutron:fip_external_mac"]; fipMAC != "" {
		extra["fip_external_mac"] = fipMAC
	}

	if len(extra) == 0 {
		return nil, nil
	}

	return &Info{Extra: extra}, nil
}
