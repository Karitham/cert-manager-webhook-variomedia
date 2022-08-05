// cert-manager webhook supporting Variomedia (https://api.variomedia.de)
//
// (c) 2022 NDE Netzdesign und -entwicklung AG, Hamburg, Germany
//
// Written by Jens-U. Mozdzen <jmozdzen@nde.ag>
//
// Licensed under Apache License 2.0 (see https://directory.fsf.org/wiki/License:Apache-2.0)
//
// Use at your own risk. As this code interacts with a paid services provider, the auther
// especially makes no claims regarding suitability of this software for your use case, nor
// that it will not cause any damage. Depending on your provider situation, using this software
// may or may not cause the use of billable services and may or may not lead to charges incurred
// to you by your service provider.
// Use at your own risk.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"context"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var GroupName = os.Getenv("GROUP_NAME")
// our DNS entry URL cache: by client domain, by entry name, by key value
var DnsEntryURL map[string]map[string]map[string]string

const (
	variomediaMinTtl = 300 // variomedia reports an error for values < this value
)

func main() {
	klog.InitFlags(nil) // initializing the klog flags
	klog.V(4).Infof( "main() called")

	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	DnsEntryURL = make(map[string]map[string]map[string]string)

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&customDNSProviderSolver{},
	)
	klog.V(4).Infof( "main() finished")
}

// customDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/jetstack/cert-manager/pkg/acme/webhook.Solver`
// interface.
type customDNSProviderSolver struct {
	client kubernetes.Clientset
}

// customDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
type customDNSProviderConfig map[string]string

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *customDNSProviderSolver) Name() string {
	return "variomedia-APIv2019"
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *customDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	klog.V(4).Infof( "Initialize() called")
	klog.V(5).InfoS("parameters", "config", kubeClientConfig)

	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	if DnsEntryURL == nil {
		DnsEntryURL = make(map[string]map[string]map[string]string)
	}

	c.client = *cl

	klog.V(4).Infof( "Initialize() finished")
	return nil
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *customDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.V(4).InfoS( "Present() called")
	klog.V(5).InfoS("parameters", "challenge", ch)

	cfg, err := c.loadApiKeys(ch.Config, ch.ResourceNamespace)
	if err != nil {
		klog.ErrorS( err, "Present() finished with error while loading API keys")
		return err
	}
        klog.V(6).Infof("decoded configuration %v", cfg)

        entry, domain, apiKey, err := c.getDomainAndEntryAndApiKey( ch, &cfg)
        if err != nil {
		klog.ErrorS( err, "Present() finished with error while determining domain and entry name")
                return fmt.Errorf("unable to get domain key for zone %s: %v", ch.ResolvedZone, err)
        }
	klog.V(4).InfoS( "present", "entry", entry, "domain", domain, "entry", entry, "API key", apiKey)

        variomediaClient := NewvariomediaClient(apiKey)

        url, err := variomediaClient.UpdateTxtRecord(&domain, &entry, &ch.Key, variomediaMinTtl)
        if err != nil {
		klog.ErrorS( err, "Present() finished with error while trying to update the DNS record")
                return fmt.Errorf("unable to change TXT record: %v", err)
        }

	// update our cache map... making sure each level of map exists
	if _, ok := DnsEntryURL[ domain]; !ok {
		DnsEntryURL[ domain] = make( map[string]map[string]string)
	}
	if _, ok := DnsEntryURL[ domain][ entry]; !ok {
		DnsEntryURL[ domain][ entry] = make( map[string]string)
	}
	DnsEntryURL[ domain][ entry][ ch.Key] = url
	klog.V(5).InfoS( "updated DNS entry cache", "cache", DnsEntryURL)

	klog.V(4).InfoS( "Present() finished")
	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *customDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	klog.V(4).InfoS( "CleanUp() called")
	klog.V(5).InfoS("parameters", "challenge", ch)

	cfg, err := c.loadApiKeys(ch.Config, ch.ResourceNamespace)
	if err != nil {
		klog.ErrorS( err, "CleanUp() finished with error while loading API keys")
		return err
	}
        klog.V(6).Infof("decoded configuration %v", cfg)

        entry, domain, apiKey, err := c.getDomainAndEntryAndApiKey( ch, &cfg)
        if err != nil {
		klog.ErrorS( err, "CleanUp() finished with error while determining domain and entry name")
                return fmt.Errorf("unable to get domain key for zone %s: %v", ch.ResolvedZone, err)
        }
	klog.V(4).InfoS( "clean up", "entry", entry, "domain", domain, "entry", entry, "API key", apiKey)

        variomediaClient := NewvariomediaClient(apiKey)

	url := DnsEntryURL[ domain][ entry][ ch.Key]

        err = variomediaClient.DeleteTxtRecord( url, variomediaMinTtl)
        if err != nil {
		klog.ErrorS( err, "CleanUp() finished with error while trying to delete the DNS record")
                return fmt.Errorf("unable to delete TXT record: %v", err)
        }

	// DNS entry deleted - so we delete our cache entry
	delete( DnsEntryURL[ domain][ entry], ch.Key)
	klog.V(5).InfoS( "updated DNS entry cache", "cache", DnsEntryURL)

	klog.V(4).InfoS( "CleanUp() finished")
	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (customDNSProviderConfig, error) {
	klog.V(4).InfoS( "loadConfig() called")

	cfg := customDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	klog.V(4).InfoS( "LoadConfig() finished")
	klog.V(5).InfoS("return values", "configuration", cfg)
	return cfg, nil
}

// loadApiKeys is a small helper function that takes the decoded JSON configuration
// and extracts the according keys.
// It's called as a wrapper to loadConfig()
func (c *customDNSProviderSolver) loadApiKeys(cfgJSON *extapi.JSON, namespace string) ( customDNSProviderConfig, error) {
	klog.V(4).InfoS( "loadApiKeys() called")
	klog.V(5).InfoS("parameters", "config", cfgJSON, "namespace", namespace)

	// retrieve configuration block from Kubernetes Issuer config
	cfg, err := loadConfig( cfgJSON)

	// no config? Abort.
	if err != nil {
		return cfg, err
	}

	for domain, secretName := range cfg {
		klog.V(6).Infof("try to load secret `%s` with key `%s`", secretName, "api-token")
		sec, err := c.client.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			klog.ErrorS( err, "loadApiKeys() finished with error")
			return nil, fmt.Errorf("unable to get secret `%s`; %v", secretName, err)
		}

		secBytes, ok := sec.Data["api-token"]
		if !ok {
			klog.ErrorS( err, "loadApiKeys() finished with error")
			return nil, fmt.Errorf("key %q not found in secret \"%s/%s\"", "api-token",
				secretName, namespace)
		}
		// replace name of secret with value of apiKey - and trim blanks and newlines
		cfg[domain] = strings.TrimRight( string(secBytes), "\r\n ")
		klog.V(6).InfoS( "stored API key", "domain", domain, "API key", cfg[domain])
	}

	klog.V(4).InfoS( "loadApiKeys() finished")
        return cfg, nil
}

// determine the appropriate domain, according API key and the actual entry to make
func (c *customDNSProviderSolver) getDomainAndEntryAndApiKey(ch *v1alpha1.ChallengeRequest, cfg *customDNSProviderConfig) (string, string, string, error) {
	klog.V(4).InfoS( "getDomainAndEntryAndApiKey() called")
	klog.V(5).InfoS("parameters", "challenge", ch, "provider config", cfg)

        // Both ch.ResolvedZone and ch.ResolvedFQDN end with a dot: '.'
        entry := strings.TrimSuffix(ch.ResolvedFQDN, ch.ResolvedZone)
        entry = strings.TrimSuffix(entry, ".")
        domain := strings.TrimSuffix(ch.ResolvedZone, ".")
        apiKey, ok := (*cfg)[domain]
        if !ok {
		klog.ErrorS( fmt.Errorf("domain '%s' not found in config.", domain), "getDomainAndEntryAndApiKey() finished with error")
                return entry, domain, apiKey, fmt.Errorf("domain '%s' not found in config.", domain)
	}

	klog.V(4).InfoS( "getDomainAndEntryAndApiKey() finished")
	klog.V(5).InfoS("return values", "entry", entry, "domain", domain, "API key", apiKey)
        return entry, domain, apiKey, nil
}

