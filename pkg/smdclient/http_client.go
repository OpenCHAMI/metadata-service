// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const smdAPIVersionPath = "/apis/smd/hsm/v2"

// HTTPClient is an SMD client backed by the SMD HTTP API.
type HTTPClient struct {
	baseURL      string
	client       *http.Client
	cache        *smdCache
	jwt          string
	tokenManager *ServiceTokenManager
	wgmu         sync.RWMutex
	wgip         map[string]string
}

// NewHTTPClient creates an SMD client backed by the SMD HTTP API.
func NewHTTPClient(baseURL, jwt string) *HTTPClient {
	return &HTTPClient{
		baseURL: normalizeBaseURL(baseURL),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: newSMDCache(time.Minute),
		jwt:   strings.TrimSpace(jwt),
		wgip:  make(map[string]string),
	}
}

// WithServiceTokenManager enables dynamic TokenSmith-backed auth for outbound SMD requests.
func (c *HTTPClient) WithServiceTokenManager(manager *ServiceTokenManager) *HTTPClient {
	c.tokenManager = manager
	return c
}

func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.Contains(trimmed, "/apis/smd/hsm/") {
		return trimmed
	}
	return trimmed + smdAPIVersionPath
}

// IDfromIP returns the component ID for a given IP address.
func (c *HTTPClient) IDfromIP(ip string) (string, error) {
	if id, ok := c.cache.getIDfromIP(ip); ok {
		return id, nil
	}

	params := url.Values{}
	params.Set("ipaddr", ip)
	var resp []compEthInterfaceV2
	if err := c.doGet("/Inventory/EthernetInterfaces", params, &resp); err != nil {
		return "", err
	}
	if len(resp) == 0 || resp[0].ComponentID == "" {
		return "", fmt.Errorf("no component found for IP %s", ip)
	}
	c.cache.setIDfromIP(ip, resp[0].ComponentID)
	return resp[0].ComponentID, nil
}

// IDfromWGIP returns the component ID for a given WireGuard IP address.
func (c *HTTPClient) IDfromWGIP(wgip string) (string, error) {
	c.wgmu.RLock()
	defer c.wgmu.RUnlock()
	for id, storedWGIP := range c.wgip {
		if storedWGIP == wgip {
			return id, nil
		}
	}
	return "", fmt.Errorf("no component found for WireGuard IP %s", wgip)
}

// IPfromID returns the IP address for a given component ID (HMN-first).
func (c *HTTPClient) IPfromID(id string) (string, error) {
	ifaces, err := c.EthernetInterfaces(id)
	if err != nil {
		return "", err
	}
	if ip := pickHMNIP(ifaces); ip != "" {
		return ip, nil
	}
	if component, err := c.ComponentInformation(id); err == nil && component.IP != "" {
		return component.IP, nil
	}
	return "", fmt.Errorf("no IP found for ID %s", id)
}

// MACfromID returns the MAC address for a given component ID (HMN-first).
func (c *HTTPClient) MACfromID(id string) (string, error) {
	ifaces, err := c.EthernetInterfaces(id)
	if err != nil {
		return "", err
	}
	if mac := pickHMNMAC(ifaces); mac != "" {
		return mac, nil
	}
	if component, err := c.ComponentInformation(id); err == nil && component.MAC != "" {
		return component.MAC, nil
	}
	return "", fmt.Errorf("no MAC found for ID %s", id)
}

// ComponentInformation returns detailed information about a component.
func (c *HTTPClient) ComponentInformation(id string) (*Component, error) {
	if component, ok := c.cache.getComponent(id); ok {
		return component, nil
	}

	path := fmt.Sprintf("/State/Components/%s", id)
	body, err := c.getRaw(path, nil)
	if err != nil {
		return nil, err
	}

	var single componentResponse
	if err := json.Unmarshal(body, &single); err == nil && single.ID != "" {
		ip := single.IP
		if ip == "" {
			ip = single.IPAddress
		}
		component := &Component{ID: single.ID, NID: single.NID, Role: single.Role, MAC: single.MAC, IP: ip}
		c.cache.setComponent(id, component)
		return component, nil
	}

	var list componentListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if len(list.Components) == 0 {
		return nil, fmt.Errorf("no component found for ID %s", id)
	}
	ip := list.Components[0].IP
	if ip == "" {
		ip = list.Components[0].IPAddress
	}
	component := &Component{ID: list.Components[0].ID, NID: list.Components[0].NID, Role: list.Components[0].Role, MAC: list.Components[0].MAC, IP: ip}
	c.cache.setComponent(id, component)
	return component, nil
}

// GroupMembership returns the list of groups a component belongs to.
func (c *HTTPClient) GroupMembership(id string) ([]string, error) {
	path := fmt.Sprintf("/memberships/%s", id)
	var resp membershipResponse
	if err := c.doGet(path, nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.GroupLabels) > 0 {
		return resp.GroupLabels, nil
	}
	if len(resp.Groups) > 0 {
		return resp.Groups, nil
	}
	return []string{}, nil
}

// AddWGIP records the allocated WireGuard IP for a component.
func (c *HTTPClient) AddWGIP(id, wgip string) error {
	c.wgmu.Lock()
	defer c.wgmu.Unlock()
	c.wgip[id] = wgip
	return nil
}

// WGIPfromID returns the stored WireGuard IP for a component.
func (c *HTTPClient) WGIPfromID(id string) (string, error) {
	c.wgmu.RLock()
	defer c.wgmu.RUnlock()
	if ip, ok := c.wgip[id]; ok {
		return ip, nil
	}
	return "", fmt.Errorf("no WGIP found for ID %s", id)
}

// EthernetNICInfo returns the list of network interfaces from RedfishSystemInfo.
func (c *HTTPClient) EthernetNICInfo(id string) ([]EthernetNIC, error) {
	if nics, ok := c.cache.getEthernetNICs(id); ok {
		return nics, nil
	}

	path := fmt.Sprintf("/Inventory/ComponentEndpoints/%s", id)
	var resp componentEndpointResponse
	if err := c.doGet(path, nil, &resp); err != nil {
		return nil, err
	}

	var nics []EthernetNIC
	if resp.RedfishSystemInfo != nil {
		for _, nic := range resp.RedfishSystemInfo.EthNICInfo {
			ifaceEnabled := false
			if nic.InterfaceEnabled != nil {
				ifaceEnabled = *nic.InterfaceEnabled
			}
			nics = append(nics, EthernetNIC{
				RedfishID:           nic.RedfishID,
				Description:         nic.Description,
				MACAddress:          nic.MACAddress,
				PermanentMACAddress: nic.PermanentMACAddress,
				InterfaceEnabled:    ifaceEnabled,
			})
		}
	}

	c.cache.setEthernetNICs(id, nics)
	return nics, nil
}

// EthernetInterfaces returns the list of EthernetInterface entries for a component.
func (c *HTTPClient) EthernetInterfaces(id string) ([]EthernetInterface, error) {
	if ifaces, ok := c.cache.getEthernetIfaces(id); ok {
		return ifaces, nil
	}

	params := url.Values{}
	params.Set("ComponentID", id)
	var resp []compEthInterfaceV2
	if err := c.doGet("/Inventory/EthernetInterfaces", params, &resp); err != nil {
		return nil, err
	}
	ifaces := make([]EthernetInterface, 0, len(resp))
	for _, iface := range resp {
		ipMappings := make([]IPMapping, 0, len(iface.IPAddresses))
		for _, ip := range iface.IPAddresses {
			ipMappings = append(ipMappings, IPMapping(ip))
		}
		ifaces = append(ifaces, EthernetInterface{
			ID:          iface.ID,
			Description: iface.Description,
			MACAddress:  iface.MACAddress,
			IPAddresses: ipMappings,
			ComponentID: iface.ComponentID,
			Type:        iface.Type,
		})
	}
	c.cache.setEthernetIfaces(id, ifaces)
	return ifaces, nil
}

func (c *HTTPClient) doGet(path string, params url.Values, out any) error {
	body, err := c.getRaw(path, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *HTTPClient) getRaw(path string, params url.Values) ([]byte, error) {
	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if c.tokenManager != nil {
		token, tokenErr := c.tokenManager.GetToken(req.Context())
		if tokenErr != nil {
			return nil, fmt.Errorf("failed to get dynamic SMD auth token: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else if c.jwt != "" {
		req.Header.Set("Authorization", "Bearer "+c.jwt)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("smd request failed: %s", strings.TrimSpace(string(body)))
	}
	return body, nil
}

type componentResponse struct {
	ID        string `json:"ID"`
	NID       int64  `json:"NID"`
	Role      string `json:"Role"`
	MAC       string `json:"MAC,omitempty"`
	IP        string `json:"IP,omitempty"`
	IPAddress string `json:"IPAddress,omitempty"`
}

type componentListResponse struct {
	Components []componentResponse `json:"Components"`
}

type membershipResponse struct {
	GroupLabels []string `json:"groupLabels"`
	Groups      []string `json:"Groups"`
}

type compEthInterfaceV2 struct {
	ID          string             `json:"ID"`
	Description string             `json:"Description"`
	MACAddress  string             `json:"MACAddress"`
	IPAddresses []compEthIPMapping `json:"IPAddresses"`
	ComponentID string             `json:"ComponentID"`
	Type        string             `json:"Type"`
}

type compEthIPMapping struct {
	IPAddress string `json:"IPAddress"`
	Network   string `json:"Network,omitempty"`
}

type componentEndpointResponse struct {
	RedfishSystemInfo *redfishSystemInfo `json:"RedfishSystemInfo"`
}

type redfishSystemInfo struct {
	EthNICInfo []ethernetNICInfo `json:"EthNICInfo"`
}

type ethernetNICInfo struct {
	RedfishID           string `json:"RedfishId"`
	Description         string `json:"Description"`
	MACAddress          string `json:"MACAddress"`
	PermanentMACAddress string `json:"PermanentMACAddress"`
	InterfaceEnabled    *bool  `json:"InterfaceEnabled"`
}

type smdCache struct {
	mu         sync.RWMutex
	ttl        time.Duration
	components map[string]cachedComponent
	ethIfaces  map[string]cachedEthernetIfaces
	ethNICs    map[string]cachedEthernetNICs
	ipToID     map[string]cachedID
}

type cachedComponent struct {
	value   *Component
	expires time.Time
}

type cachedEthernetIfaces struct {
	value   []EthernetInterface
	expires time.Time
}

type cachedEthernetNICs struct {
	value   []EthernetNIC
	expires time.Time
}

type cachedID struct {
	value   string
	expires time.Time
}

func newSMDCache(ttl time.Duration) *smdCache {
	return &smdCache{
		ttl:        ttl,
		components: make(map[string]cachedComponent),
		ethIfaces:  make(map[string]cachedEthernetIfaces),
		ethNICs:    make(map[string]cachedEthernetNICs),
		ipToID:     make(map[string]cachedID),
	}
}

func (c *smdCache) getComponent(id string) (*Component, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.components[id]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	copy := *entry.value
	return &copy, true
}

func (c *smdCache) setComponent(id string, component *Component) {
	if component == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := *component
	c.components[id] = cachedComponent{value: &copy, expires: time.Now().Add(c.ttl)}
}

func (c *smdCache) getEthernetIfaces(id string) ([]EthernetInterface, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.ethIfaces[id]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	result := make([]EthernetInterface, len(entry.value))
	copy(result, entry.value)
	return result, true
}

func (c *smdCache) setEthernetIfaces(id string, ifaces []EthernetInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyIfaces := make([]EthernetInterface, len(ifaces))
	copy(copyIfaces, ifaces)
	c.ethIfaces[id] = cachedEthernetIfaces{value: copyIfaces, expires: time.Now().Add(c.ttl)}
}

func (c *smdCache) getEthernetNICs(id string) ([]EthernetNIC, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.ethNICs[id]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	result := make([]EthernetNIC, len(entry.value))
	copy(result, entry.value)
	return result, true
}

func (c *smdCache) setEthernetNICs(id string, nics []EthernetNIC) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyNics := make([]EthernetNIC, len(nics))
	copy(copyNics, nics)
	c.ethNICs[id] = cachedEthernetNICs{value: copyNics, expires: time.Now().Add(c.ttl)}
}

func (c *smdCache) getIDfromIP(ip string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.ipToID[ip]
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.value, true
}

func (c *smdCache) setIDfromIP(ip, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ipToID[ip] = cachedID{value: id, expires: time.Now().Add(c.ttl)}
}
