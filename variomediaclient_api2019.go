// client implementation for Variomedia API, 2019+ version (https://api.variomedia.de/docs/)
//
// Written by Jens-Uwe Mozdzen <jmozdzen@nde.ag>
//
// Licensed under LGPL v3
//
// Entry points:
// client = NewvariomediaClient( apikey)
//	- create new instance of API client
//	in:
//		apikey	-	customer-specific API key issued by Variomedia
//	returns:
//		client object
//
//
// client.UpdateTxtRecord(&domain, &entry, Key, ttl)
//	- update TXT record
//	in:
//		domain	-	DNS domain
//		entry	-	host label
//		key	-	valus of TXT record
//		ttl	-	TTL of record
//	returns:
//		variomediaDNSEntryURL   -       the URL of the resulting DNS entry
//
// client.DeleteTxtRecord(&domain, &entry)
//	- delete TXT record for entry/domain
//	in:
//		url     -       DNS entry's URL
//		ttl     -       TTL of record
//	returns:
//		-

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

const (
	variomediaLiveDnsBaseUrl = "https://api.variomedia.de/dns-records"
	statusLookupDelay = 2 * time.Second
)

type variomediaClient struct {
	apiKey              string
}

type variomediaDnsAttributes struct {
	RecordType	string	`json:"record_type"`
	Name		string	`json:"name"`
	Domain		string	`json:"domain"`
	Data		string	`json:"data"`
	Ttl		int	`json:"ttl"`
} // variomediaDnsAttributes

type variomediaData struct {
	RequestType	string	`json:"type"`
	RequestAttr	variomediaDnsAttributes	`json:"attributes"`
} // variomediaData

type variomediaRequest struct {
	variomediaData `json:"data"`
} // variomediaRequest

type variomediaJobData struct {
	Type	string `json:"type"`
	Id	string `json:"id"`
	Attributes	map[string]string `json:"attributes"`
	Links	map[string]string `json:"links"`
} // variomediaJobData

type variomediaResponse struct {
	Data variomediaJobData `json:"data"`
	Links map[string]string `json:"links"`
} // variomediaRequest

// NewvariomediaClient()
// create new instance of Variomedia client
func NewvariomediaClient(apiKey string) *variomediaClient {
	klog.V(4).InfoS("NewvariomediaClient() called")
	klog.V(5).InfoS("parameters", "API key", apiKey)

	klog.V(4).InfoS("NewvariomediaClient() finished")
	return &variomediaClient{
		apiKey:              apiKey,
	}
}

// client.UpdateTxtRecord(&domain, &entry, Key, ttl)
//	- create or update TXT record
//	in:
//		domain	-	DNS domain
//		entry	-	host label
//		key	-	valus of TXT record
//		ttl	-	TTL of record
//	returns:
//		variomediaDNSEntryURL	-	the URL of the resulting DNS entry
func (c *variomediaClient) UpdateTxtRecord(domain *string, name *string, value *string, ttl int) (string, error) {
	klog.V(4).InfoS("UpdateTxtRecord() called")
	klog.V(5).InfoS("parameters", "domain", *domain, "name", *name, "value", *value, "TTL", ttl)

	var reqData variomediaRequest

	reqData = variomediaRequest{ variomediaData{
						RequestType: "dns-record",
						RequestAttr: variomediaDnsAttributes{
							RecordType: "TXT",
							Name: *name,
							Domain: *domain,
							Data: *value,
							Ttl: ttl,
						},
					},
				}

	// the actual request is encoded in JSON
	body, err := json.Marshal( reqData)
	if err != nil {
		klog.ErrorS(err, "UpdateTxtRecord() finished with error")
		return "", fmt.Errorf("cannot marshall to json: %v", err)
	}

	req, err := http.NewRequest("POST", variomediaLiveDnsBaseUrl, bytes.NewReader(body))
	if err != nil {
		klog.ErrorS(err, "UpdateTxtRecord() finished with error")
		return "", err
	}

	// contact Variomedia and check the results
	status, respData, err := c.doRequest(req, true)
	if err != nil {
		klog.ErrorS(err, "UpdateTxtRecord() finished with error")
		return "", err
	}

	// have we hit the rate limit?
	if status == http.StatusTooManyRequests {
		klog.ErrorS( nil, "UpdateTxtRecord() finished with errori 'too many requests' reported by Variomedia")
		return "", fmt.Errorf("Variomedia rate limit reached (HTTP code %d)", http.StatusTooManyRequests)
	}

	if status != http.StatusCreated && status != http.StatusOK && status != http.StatusAccepted {
		klog.ErrorS(nil, "UpdateTxtRecord() finished with error reported by server", "status code", status)
		return "", fmt.Errorf("failed creating TXT record: server reported status code %d", status)
	}

	// the request has succeeded - but is the job already finished?
	// check the response for an according '' element
	var reply variomediaResponse
	err = json.Unmarshal( respData, &reply)
	if err != nil {
		klog.ErrorS(err, "UpdateTxtRecord() finished with error")
		return "", fmt.Errorf("cannot unmarshall response to json: %v", err)
	}
	klog.V(5).InfoS( "HTTP finished", "JSON reply", reply)

	// we give it 5 iterations to finish
	loopcount := 5
	for {
		if reply.Data.Attributes[ "status"] == "pending" {
			klog.V(2).InfoS( "DNS job still pending")

			// inter-loop delay
			time.Sleep( statusLookupDelay)

			// re-fetch the job status
			req, err := http.NewRequest("GET", reply.Data.Links[ "queue-job"], nil)
			if err != nil {
				klog.ErrorS(err, "UpdateTxtRecord() finished with error")
				return "", err
			}

			// contact Variomedia and check the results
			status, respData, err := c.doRequest(req, true)
			if err != nil {
				klog.ErrorS(err, "UpdateTxtRecord() finished with error")
				return "", err
			}

			// have we hit the rate limit?
			if status == http.StatusTooManyRequests {
				klog.ErrorS( nil, "UpdateTxtRecord() finished with errori 'too many requests' reported by Variomedia")
				return "", fmt.Errorf("Variomedia rate limit reached (HTTP code %d)", http.StatusTooManyRequests)
			}

			if status != http.StatusCreated && status != http.StatusOK && status != http.StatusAccepted {
				klog.ErrorS(nil, "UpdateTxtRecord() finished with error reported by server", "status code", status)
				return "", fmt.Errorf("failed creating TXT record: server reported status code %d", status)
			}

			// the request has succeeded - but is the job already finished?
			// check the response for an according '' element
			err = json.Unmarshal( respData, &reply)
			if err != nil {
				klog.ErrorS(err, "UpdateTxtRecord() finished with error")
				return "", fmt.Errorf("cannot unmarshall response to json: %v", err)
			}
			klog.V(5).InfoS( "HTTP finished", "JSON reply", reply)

		}
		loopcount -= 1

		if (reply.Data.Attributes[ "status"] == "done") {
			klog.V(2).InfoS( "DNS job finished", "retries left", loopcount)
			break;
		}
		if (loopcount == 0) {
			klog.ErrorS(nil, "UpdateTxtRecord() finished with error: job timed out", "most recent status", reply.Data.Attributes[ "status"])
			return "", fmt.Errorf("DNS update job timed out with most recent status '%s'", reply.Data.Attributes[ "status"])
		}
	} // emulated do until

	klog.V(4).InfoS("UpdateTxtRecord() finished")
	klog.V(5).InfoS("return values", "url", reply.Data.Links[ "dns-record"])
	return reply.Data.Links[ "dns-record"], nil
} //func UpdateTxtRecord()

// client.DeleteTxtRecord(&domain, &entry, Key, ttl)
//	- create or update TXT record
//	in:
//		url	-	DNS entry's URL
//		ttl	-	TTL of record
//	returns:
//		-
func (c *variomediaClient) DeleteTxtRecord(url string, ttl int) error {
	klog.V(4).InfoS("DeleteTxtRecord() called")
	klog.V(5).InfoS("parameters", "url", url, "TTL", ttl)

	// deleting a record happens by sending a HTTP "DELETE" request to the DNS entry's URL
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		klog.ErrorS(err, "DeleteTxtRecord() finished with error")
		return err
	}

	// contact Variomedia and check the results
	status, respData, err := c.doRequest(req, true)
	if err != nil {
		klog.ErrorS(err, "DeleteTxtRecord() finished with error")
		return err
	}

	// have we hit the rate limit?
	if status == http.StatusTooManyRequests {
		klog.ErrorS( nil, "DeleteTxtRecord() finished with errori 'too many requests' reported by Variomedia")
		return fmt.Errorf("Variomedia rate limit reached (HTTP code %d)", http.StatusTooManyRequests)
	}

	if status != http.StatusCreated && status != http.StatusOK && status != http.StatusAccepted {
		klog.ErrorS(nil, "DeleteTxtRecord() finished with error reported by server", "status code", status)
		return fmt.Errorf("failed deleting TXT record: %v", err)
	}

	// the request has succeeded - but is the job already finished?
	// check the response for an according '' element
	var reply variomediaResponse
	err = json.Unmarshal( respData, &reply)
	if err != nil {
		klog.ErrorS(err, "DeleteTxtRecord() finished with error")
		return fmt.Errorf("cannot unmarshall response to json: %v", err)
	}
	klog.V(5).InfoS( "HTTP finished", "JSON reply", reply)

	// we give it 5 iterations to finish
	loopcount := 5
	for {
		if reply.Data.Attributes[ "status"] == "pending" && status != http.StatusNotFound {
			klog.V(2).InfoS( "DNS job still pending")

			// inter-loop delay: two seconds
			time.Sleep( statusLookupDelay)

			// re-fetch the job status
			req, err := http.NewRequest("GET", reply.Data.Links[ "queue-job"], nil)
			if err != nil {
				klog.ErrorS(err, "DeleteTxtRecord() finished with error")
				return err
			}

			// contact Variomedia and check the results
			status, respData, err := c.doRequest(req, true)
			if err != nil {
				klog.ErrorS(err, "DeleteTxtRecord() finished with error")
				return err
			}

			// have we hit the rate limit?
			if status == http.StatusTooManyRequests {
				klog.ErrorS( nil, "DeleteTxtRecord() finished with errori 'too many requests' reported by Variomedia")
				return fmt.Errorf("Variomedia rate limit reached (HTTP code %d)", http.StatusTooManyRequests)
			}

			if status != http.StatusCreated && status != http.StatusOK && status != http.StatusAccepted && status != http.StatusNotFound {
				klog.ErrorS(nil, "DeleteTxtRecord() finished with error reported by server", "status code", status)
				return fmt.Errorf("failed creating TXT record: %v", err)
			}

			// the request has succeeded - but is the job already finished?
			// check the response for an according '' element
			err = json.Unmarshal( respData, &reply)
			if err != nil {
				klog.ErrorS(err, "DeleteTxtRecord() finished with error")
				return fmt.Errorf("cannot unmarshall response to json: %v", err)
			}
			klog.V(5).InfoS( "HTTP finished", "JSON reply", reply)

		}

		// 404 means "DNS record not found" (anymore) - we're fine with that, the record is gone
		if status == http.StatusNotFound {
			klog.V(4).InfoS("DeleteTxtRecord() finished because DNS record is gone", "retries left", loopcount)
			return nil
		}

		loopcount -= 1

		if (reply.Data.Attributes[ "status"] == "done") {
			klog.V(2).InfoS( "DNS job finished", "retries left", loopcount)
			break;
		}
		if (loopcount == 0) {
			klog.ErrorS(nil, "DeleteTxtRecord() finished with error: job timed out", "most recent status", reply.Data.Attributes[ "status"])
			return fmt.Errorf("DNS update job timed out with most recent status '%s'", reply.Data.Attributes[ "status"])
		}
	} // emulated do until

	klog.V(4).InfoS("DeleteTxtRecord() finished")
	return nil
} // func DeleteTxtRecord()


func (c *variomediaClient) variomediaRecordsUrl(domain string) string {
	klog.V(4).InfoS("variomediaRecordsUrl() called")
	klog.V(5).InfoS("parameters", "domain", domain)

	urlpart := fmt.Sprintf("%s/dns-records", domain)

	klog.V(4).InfoS("variomediaRecordsUrl() finished")
	klog.V(5).InfoS("return values", "url part", urlpart)
	return urlpart
}

func (c *variomediaClient) doRequest(req *http.Request, readResponseBody bool) (int, []byte, error) {
	klog.V(4).InfoS("doRequest() called")
	klog.V(5).InfoS("parameters", "request", req, "readResponseBody", readResponseBody)

	// Variomedia uses headers for auth, request content type and to signal accepted API versions
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.apiKey))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.variomedia.v1+json")

	client := http.Client{
		Timeout: 30 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		klog.ErrorS(err, "doRequest() finished with error")
		return 0, nil, err
	}

	klog.V(5).InfoS( "HTTP request", "response", res)

	// check for proper returns
	if (res.StatusCode == http.StatusOK || res.StatusCode == http.StatusAccepted) && readResponseBody {
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			klog.ErrorS(err, "HTTP request finished with error")
			return 0, nil, err
		}
		klog.V(4).InfoS( "HTTP request succeeded", "status code", res.StatusCode)
		klog.V(5).InfoS( "HTTP request result", "data", data)
		return res.StatusCode, data, nil
	}

	klog.V(4).InfoS("doRequest() finished")
	klog.V(5).InfoS("return values", "status code", res.StatusCode)
	return res.StatusCode, nil, nil
}
