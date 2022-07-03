# Solver testdata directory

The cert-manager team has provided an "integrated" testing mechanism with their
"example" implementation of the web hook. It can be called via the provided
"makefile" and needs configuration data so the connection to Variomedia can be
tested live. The according config data goes into this directory.

At least two files need to be provided:

- a config file, defining the web hook config. A sample version of the file
  is provided, you can rename "config.json.sample" to "config.json" and adjust
  the definition provided, to point to your Variomedia domain(s) and according
  secret(s).

- a secret's manifest prviding the API key under the secret name you configured
  in your config.json. Two files "variomedia-credentials-0[12].yaml.sample" are
  provided, you can rename these to "*.yaml" to create secrets matching the sample
  config in config.json

The sample file are configured to run three domains using two API keys - of course,
the keys contained in the sample secrets are *not* live Variomedia secrets and
will cause the tests to fail.

## creating your own config

The content of "config.json" represents the part of the later "Issuer" configuration
and as the name implies, needs to be in JSON format.

If you are the owner of a domain "myvariomediadomain.com" and intend to provide the
according API key in a secret called "variomedia-secret", the JSON file needs to
look like
```
{
    "myvariomediadomain.com": "variomedia-secret"
}
```

## creating your own secret

The secret manifest will be used by the testing code to create the mandatory
secrets containing the encoded Variomedia API keys. While the file names can
be of your choice (keep the extension ".yaml", though), the name of the secret
needs to match the name given in config.json
You can create the according base64-encoded string via

```
#  echo -n "YourVariomediaAPIKeyGoesHere" | base64
WW91clZhcmlvbWVkaWFBUElLZXlHb2VzSGVyZQ==
```

The implementation of this web hook is removing any trailing blanks, new-lines
and carriage-returns. Therefore, you can also use the following call to create
the base64-encoded string:

```
#  base64 <<< "YourVariomediaAPIKeyGoesHere"
WW91clZhcmlvbWVkaWFBUElLZXlHb2VzSGVyZQo=
```


config.json  config.json.sample  README.md  variomedia-credentials-01.yaml.sample  variomedia-credentials-02.yaml.sample  variomedia-credentials-ndeag.yaml

