package enrich

import (
	"context"
	"fmt"

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

	provider, err := openstack.AuthenticatedClient(ctx, opts)
	if err != nil {
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

func (*OpenStackProvider) Name() string { return "openstack" }

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
