// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/OpenCHAMI/cloud-init/pkg/resources/group"
	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Store defines the interface for data storage operations
type Store interface {
	GetClusterDefaults() (*ClusterDefaults, error)
	GetInstanceInfo(id string) (*InstanceInfo, error)
	GetGroupData(name string) (*group.Group, error)
}

// ClusterDefaults holds cluster-wide default configuration
type ClusterDefaults struct {
	BaseURL          string   `json:"base_url"`
	CloudProvider    string   `json:"cloud_provider"`
	Region           string   `json:"region"`
	AvailabilityZone string   `json:"availability_zone"`
	ClusterName      string   `json:"cluster_name"`
	ShortName        string   `json:"short_name"`
	NidLength        int      `json:"nid_length"`
	PublicKeys       []string `json:"public_keys"`
}

// InstanceInfo holds instance-specific configuration
type InstanceInfo struct {
	InstanceID       string   `json:"instance_id"`
	LocalHostname    string   `json:"local_hostname"`
	Hostname         string   `json:"hostname"`
	CloudInitBaseURL string   `json:"cloud_init_base_url"`
	PublicKeys       []string `json:"public_keys"`
}

// MetaData represents the metadata structure returned to cloud-init clients
type MetaData struct {
	InstanceID    string       `json:"instance-id" yaml:"instance-id"`
	LocalHostname string       `json:"local-hostname" yaml:"local-hostname"`
	Hostname      string       `json:"hostname" yaml:"hostname"`
	ClusterName   string       `json:"cluster-name" yaml:"cluster-name"`
	InstanceData  InstanceData `json:"instance-data" yaml:"instance_data"`
}

// InstanceData contains detailed instance information for cloud-init
type InstanceData struct {
	V1 struct {
		CloudName        string     `json:"cloud-name,omitempty" yaml:"cloud_name,omitempty"`
		AvailabilityZone string     `json:"availability-zone,omitempty" yaml:"availability_zone,omitempty"`
		InstanceID       string     `json:"instance-id,omitempty" yaml:"instance_id,omitempty"`
		InstanceType     string     `json:"instance-type,omitempty" yaml:"instance_type,omitempty"`
		LocalHostname    string     `json:"local-hostname,omitempty" yaml:"local_hostname,omitempty"`
		Region           string     `json:"region,omitempty" yaml:"region,omitempty"`
		Hostname         string     `json:"hostname,omitempty" yaml:"hostname,omitempty"`
		LocalIPv4        string     `json:"local-ipv4,omitempty" yaml:"local_ipv4,omitempty"`
		CloudProvider    string     `json:"cloud-provider,omitempty" yaml:"cloud_provider,omitempty"`
		PublicKeys       []string   `json:"public-keys,omitempty" yaml:"public_keys,omitempty"`
		VendorData       VendorData `json:"vendor-data,omitempty" yaml:"vendor_data,omitempty"`
	} `json:"v1" yaml:"v1"`
}

// VendorData contains vendor-specific metadata
type VendorData struct {
	Version          string                    `json:"version" yaml:"version"`
	CloudInitBaseURL string                    `json:"cloud_init_base_url,omitempty" yaml:"cloud_init_base_url,omitempty"`
	ClusterName      string                    `json:"cluster_name,omitempty" yaml:"cluster_name,omitempty"`
	Nid              int64                     `json:"nid,omitempty" yaml:"nid,omitempty"`
	Role             string                    `json:"role,omitempty" yaml:"role,omitempty"`
	Groups           map[string]map[string]any `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// getActualRequestIP extracts the real client IP from the request,
// handling X-Forwarded-For headers for proxy scenarios
func getActualRequestIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// MetaDataHandler returns metadata for the requesting node based on its IP address
func MetaDataHandler(smd smdclient.SMDClient, store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get component ID from requesting IP
		ip := getActualRequestIP(r)
		log.Debug().Msgf("Metadata request from IP: %s", ip)

		id, err := smd.IDfromIP(ip)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get component ID from IP %s", ip)
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}

		log.Debug().Msgf("Getting metadata for component: %s", id)

		// Get component information
		component, err := smd.ComponentInformation(id)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get component information for %s", id)
			http.Error(w, "component information not available", http.StatusInternalServerError)
			return
		}

		// Get group memberships
		groups, err := smd.GroupMembership(id)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to get group membership for %s, continuing with empty groups", id)
			groups = []string{}
		}

		// Get boot IP and MAC
		bootIP, _ := smd.IPfromID(id)
		bootMAC, _ := smd.MACfromID(id)

		// Generate metadata
		metadata := generateMetaData(component, groups, bootIP, bootMAC, store)

		// Return as YAML
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)

		yamlData, err := yaml.Marshal(metadata)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal metadata to YAML")
			http.Error(w, "failed to encode metadata", http.StatusInternalServerError)
			return
		}

		if _, err = w.Write(yamlData); err != nil {
			log.Error().Err(err).Msg("Failed to write response")
		}
	}
}

// generateMetaData creates the metadata structure from component and storage data
func generateMetaData(component *smdclient.Component, groups []string, bootIP, bootMAC string, store Store) MetaData {
	metadata := MetaData{}

	// Get cluster defaults
	clusterDefaults, err := store.GetClusterDefaults()
	if err != nil || clusterDefaults == nil {
		log.Warn().Err(err).Msg("Failed to get cluster defaults, using empty values")
		clusterDefaults = &ClusterDefaults{}
	}

	// Get instance-specific info
	instanceInfo, err := store.GetInstanceInfo(component.ID)
	if err != nil || instanceInfo == nil {
		log.Debug().Err(err).Msgf("No instance info for %s, using defaults", component.ID)
		instanceInfo = &InstanceInfo{}
	}

	// Set instance ID
	if instanceInfo.InstanceID != "" {
		metadata.InstanceID = instanceInfo.InstanceID
	} else {
		metadata.InstanceID = component.ID
	}

	// Set hostnames
	if instanceInfo.LocalHostname != "" {
		metadata.LocalHostname = instanceInfo.LocalHostname
	} else {
		metadata.LocalHostname = generateHostname(clusterDefaults.ClusterName, clusterDefaults.ShortName, clusterDefaults.NidLength, component)
	}

	if instanceInfo.Hostname != "" {
		metadata.Hostname = instanceInfo.Hostname
	} else {
		metadata.Hostname = generateHostname(clusterDefaults.ClusterName, clusterDefaults.ShortName, clusterDefaults.NidLength, component)
	}

	metadata.ClusterName = clusterDefaults.ClusterName

	// Build instance data
	instanceData := InstanceData{}
	instanceData.V1.CloudName = "OpenCHAMI"
	instanceData.V1.CloudProvider = clusterDefaults.CloudProvider
	instanceData.V1.Region = clusterDefaults.Region
	instanceData.V1.AvailabilityZone = clusterDefaults.AvailabilityZone
	instanceData.V1.InstanceID = metadata.InstanceID
	instanceData.V1.LocalHostname = metadata.LocalHostname
	instanceData.V1.Hostname = metadata.Hostname
	instanceData.V1.LocalIPv4 = bootIP

	// Merge public keys
	instanceData.V1.PublicKeys = append(clusterDefaults.PublicKeys, instanceInfo.PublicKeys...)

	// Build vendor data
	instanceData.V1.VendorData.Version = "1.0"
	instanceData.V1.VendorData.ClusterName = clusterDefaults.ClusterName
	instanceData.V1.VendorData.Nid = component.NID
	instanceData.V1.VendorData.Role = component.Role

	// Set cloud-init base URL
	if instanceInfo.CloudInitBaseURL != "" {
		instanceData.V1.VendorData.CloudInitBaseURL = instanceInfo.CloudInitBaseURL
	} else {
		instanceData.V1.VendorData.CloudInitBaseURL = clusterDefaults.BaseURL
	}

	// Add group data to vendor data
	if len(groups) > 0 {
		instanceData.V1.VendorData.Groups = make(map[string]map[string]any)
		for _, groupName := range groups {
			groupData, err := store.GetGroupData(groupName)
			if err != nil {
				log.Warn().Err(err).Msgf("Failed to get data for group %s", groupName)
				instanceData.V1.VendorData.Groups[groupName] = map[string]any{
					"description": "No description found",
				}
				continue
			}

			// Add group metadata
			groupMeta := make(map[string]any)
			groupMeta["description"] = groupData.Spec.Description
			for k, v := range groupData.Spec.MetaData {
				groupMeta[k] = v
			}
			instanceData.V1.VendorData.Groups[groupName] = groupMeta
		}
	}

	metadata.InstanceData = instanceData
	return metadata
}

// generateHostname creates a hostname from cluster name and component NID
func generateHostname(clusterName, shortName string, nidLength int, component *smdclient.Component) string {
	var sname string
	var nlen int

	if shortName == "" {
		if len(clusterName) >= 2 {
			sname = clusterName[:2]
		} else {
			sname = clusterName
		}
	} else {
		sname = shortName
	}

	if nidLength == 0 {
		nlen = 4
	} else {
		nlen = nidLength
	}

	return fmt.Sprintf("%s%0*d", sname, nlen, component.NID)
}

// UserDataHandler returns user-data for the requesting node
// For OpenCHAMI, this is always blank to preserve user override capability
func UserDataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/cloud-config")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("#cloud-config\n")); err != nil {
		log.Error().Err(err).Msg("Failed to write response")
	}
}

// VendorDataHandler returns vendor-data as an include-file list
func VendorDataHandler(smd smdclient.SMDClient, store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get component ID from requesting IP
		ip := getActualRequestIP(r)
		log.Debug().Msgf("Vendor-data request from IP: %s", ip)

		id, err := smd.IDfromIP(ip)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get component ID from IP %s", ip)
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}

		// Get group memberships
		groups, err := smd.GroupMembership(id)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to get group membership for %s, returning empty include list", id)
			groups = []string{}
		}

		// Get base URL
		clusterDefaults, err := store.GetClusterDefaults()
		baseURL := ""
		if err == nil {
			baseURL = clusterDefaults.BaseURL
		}

		// Check for instance-specific override
		instanceInfo, err := store.GetInstanceInfo(id)
		if err == nil && instanceInfo.CloudInitBaseURL != "" {
			baseURL = instanceInfo.CloudInitBaseURL
		}

		// Build include list
		payload := "#include\n"
		for _, groupName := range groups {
			payload += fmt.Sprintf("%s/%s.yaml\n", baseURL, groupName)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(payload)); err != nil {
			log.Error().Err(err).Msg("Failed to write response")
		}
	}
}

// GroupUserDataHandler returns group-specific cloud-config with template rendering
func GroupUserDataHandler(smd smdclient.SMDClient, store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupName := chi.URLParam(r, "group")

		// Get component ID from requesting IP
		ip := getActualRequestIP(r)
		log.Debug().Msgf("Group user-data request from IP: %s for group: %s", ip, groupName)

		id, err := smd.IDfromIP(ip)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get component ID from IP %s", ip)
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}

		// Verify node is member of group
		groups, err := smd.GroupMembership(id)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get group membership for %s", id)
			http.Error(w, "failed to verify group membership", http.StatusInternalServerError)
			return
		}

		isMember := false
		for _, g := range groups {
			if g == groupName {
				isMember = true
				break
			}
		}

		if !isMember {
			log.Warn().Msgf("Node %s is not a member of group %s", id, groupName)
			http.Error(w, fmt.Sprintf("node %s is not a member of group %s", id, groupName), http.StatusNotFound)
			return
		}

		// Get group data
		groupData, err := store.GetGroupData(groupName)
		if err != nil {
			log.Warn().Err(err).Msgf("No data for group %s, returning empty cloud-config", groupName)
			w.Header().Set("Content-Type", "text/cloud-config")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("#cloud-config\n"))
			return
		}

		// Get component info for metadata context
		component, err := smd.ComponentInformation(id)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get component information for %s", id)
			http.Error(w, "component information not available", http.StatusInternalServerError)
			return
		}

		// Get cluster defaults for merging
		clusterDefaults, err := store.GetClusterDefaults()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get cluster defaults")
			clusterDefaults = &ClusterDefaults{}
		}

		// Build metadata context for template rendering
		defaultMeta := map[string]string{
			"hostname":    generateHostname(clusterDefaults.ClusterName, clusterDefaults.ShortName, clusterDefaults.NidLength, component),
			"instance_id": component.ID,
			"nid":         fmt.Sprintf("%d", component.NID),
			"role":        component.Role,
		}

		// Merge with group metadata
		merged := group.MergeMetadata(defaultMeta, groupData.Spec.MetaData)

		// Render template
		rendered, err := group.RenderTemplate(groupData.Spec.Template, merged)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to render template for group %s", groupName)
			http.Error(w, "template rendering failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/cloud-config")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(rendered)); err != nil {
			log.Error().Err(err).Msg("Failed to write response")
		}
	}
}
