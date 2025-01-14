<p align="center">
  <img src="https://raw.githubusercontent.com/cert-manager/cert-manager/d53c0b9270f8cd90d908460d69502694e1838f5f/logo/logo-small.png" height="256" width="256" alt="cert-manager project logo" />
</p>

# ACME webhook for Variomedia AG (post-2018 API)

The ACME issuer type supports an optional 'webhook' solver, which can be used
to implement custom DNS01 challenge solving logic.

This is useful if you need to use cert-manager with a DNS provider that is not
officially supported in cert-manager core. This project "cert-manager-webhook-variomedia"
implements a webhook to be used with Variomedia's new (post-2018) API for updating
DNS entries. Support for Variomedia's legacy API can be found in https://github.com/jheyduk/cert-manager-webhook-variomedia

You can find Variomedia's documentation on the "new" API in https://api.variomedia.de/docs/
(German language only) - please don't confuse this with the legacy "Reseller API", see 
https://api.variomedia.de/docs/legacy/ .

# Security warning

The API keys provided by Variomedia are currently **not** restrictable to allow for
DNS updates only - if your key is compromised, **any** entry in your Variomedia customer
profile can be updated by the one having the key.

Also note that you are solely responsible for protecting access to not only the key, but also
to the running webhook: Anyone with access to the webhook will be able to update your DNS entries
at Variomedia, including adding malicious entries, overriding existing entries (even if not DNS-01-related)
and deleting existing entries (even if not DNS-01-related).

By using this software, you agree to not hold responsible the authors of this software
for **any** damage that may occur to you, directly or indirectly, and accept that the
authors of this software make no guarantees on the suitability of this software for any use.

In other words: Use this software at your own risk.

If you find security flaws in the implementation of this software, please report an
according issue at https://github.com/jmozd/cert-manager-webhook-variomedia/issue .

## Development & building

### Origins of the Variomedia webhook

Webhook's themselves are deployed as Kubernetes API services, in order to allow
administrators to restrict access to webhooks with Kubernetes RBAC.

This is important, as otherwise it'd be possible for anyone with access to your
webhook to complete ACME challenge validations and obtain certificates.

The Variomedia AG webhook implementation is based on the example webhook provided
by the cert-manager project (https://github.com/cert-manager/webhook-example). Also,
inspiration was taken from an implementation for the old Variomedia "provider API",
which can be found at https://github.com/jheyduk/cert-manager-webhook-variomedia.

### Using your own repository

The GitHub version of the Variomedia webhook implementation is currently focussed on providing
an implementation in a decentral container registry, i.e. "Harbor". The Docker image
is currently *not* published on docker.io. This may change at a later time.

#### Running the test suite

**It is essential that you configure and run the test suite after modifying the
DNS01 webhook.**

You can run the test suite with:

```bash
$ TEST_ZONE_NAME=example.com. make test
```

Setting the trailing "." on the zone name (for which you have the Variomedia API key
and set up the files in the testdata/my-custom-solver/ subdirectory) is required, the
test run might otherwise fail.

### Pushing the Docker image
Once you have your registry up & running (which is not part of this README description),
you can build and upload your local copy of the software using the following commands:

```bash
# to upload the container image to your registry
$ export REGISTRY='your.registry.company.com/yourproject'
$ docker login $REGISTRY

# build and push the resulting image to your repository
# will invoke via dependencies test -> build -> push
$ TEST_ZONE_NAME=example.com. make push
```

## Installation via Helm chart

We have provided a Helm chart to ease the installation of the Variomedia webhook.

When specifying the groupName parameter, make sure to use a name in your cluster's domain.
If you set that differently from "cluster.local", you'll need to use the proper domain suffix
both as a Helm value and when creating the (Cluster)Issuer (see below).

## Configuration

In addition to installing the webhook, you will also need to configure it and create at least one
cert-manager Issuer.

Configuration of the webhook consists in providing the according secrets for each DNS domain you
intend to generate certificates for (via cert-manager and i.e. "Let's Encrypt!"). This is done by creating
a Kubernetes "secret" for each API key issued by Variomedia to you and then configuring the cert-manager
to reference each according secret per DNS domain handled by the Issuer:

```bash
$ kubectl create secret generic variomedia-credentials-01 --from-literal=api-token='yourApiKeyGoesHere'
$ kubectl create secret generic variomedia-credentials-02 --from-literal=api-token='someOtherApiKeyGoesHere'
$ kubectl apply -f - << EOF
        apiVersion: cert-manager.io/v1
        kind: ClusterIssuer
        metadata:
          name: letsencrypt-staging
          namespace: cert-manager
        spec:
          acme:
            # The ACME test server URL
            server: https://acme-staging-v02.api.letsencrypt.org/directory
            # The ACME production server URL
            #server: https://acme-v02.api.letsencrypt.org/directory

            # Email address used for ACME registration
            email: yourEmailAsKnownToLE@company.com

            # Name of a secret used to store the ACME account private key
            privateKeySecretRef:
              name: letsencrypt-staging
            solvers:
            - dns01:
                webhook:
                  groupName: cert-manager-webhook-variomedia.cluster.local
                  solverName: variomedia-APIv2019
                  config:
                    example.com: variomedia-credentials-01
                    someotherdomain.com: variomedia-credentials-01
                    somethirddomain.com: variomedia-credentials-02
EOF
```

Although three domains were covered in above example, typically you'll have only a single domain to configure - you then can
omit creating "secret/variomedia-credentials-02" and will have to specify only a single entry in "...:webhook:config".

Variomedia AG published a page describing how to obtain the according API key (the page is in German
only), basically stating that you can contact their support to have a key issued:
https://www.variomedia.de/faq/Wie-bekomme-ich-einen-API-Token/article/326

Please report any problems and errors you experience by using this webhook, via https://github.com/jmozd/cert-manager-webhook-variomedia/issues

