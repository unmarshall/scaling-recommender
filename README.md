# scaling-recommender

[![reuse compliant](https://reuse.software/badge/reuse-compliant.svg)](https://reuse.software/)

:construction_worker: Under construction.

> [NOTE]
> This is prototype for Proof of Concept only.

## Maintain copyright and license information
By default all source code files are under `Apache 2.0` and all markdown files are under `Creative Commons` license.

When creating new source code files the license and copyright information should be provided using corresponding SPDX headers.

Example for go source code files (replace `<year>` with the current year)
```
/*
 * SPDX-FileCopyrightText: <year> SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */
```

## What is scaling recommender? 

1. Scaling recommender is an application that gives scale up recommendations for a given http input request. 
1. The http input request body is a `clusterSnapshot` defined in https://github.com/elankath/gardener-scaling-common/blob/main/types.go.  
1. The output is a list of recommendation objects each consisting of a `WokerPoolName`, a `Zone` and an `IncrementBy` field.

1. Scaling recommender also applies its recommendation on the target cluster by using the kubeconfig specified in the `target-kvcl-kubeconfig`
command line flag. The kubeconfig specified is ideally a kubeconfig of a virtual cluster like one setup by https://github.com/unmarshall/kvcl/.

## Launch the Scaling Recommender

To Launch the recommender, execute the following command:
    
```bash
go run main.go --target-kvcl-kubeconfig <path-to-kubeconfig> --provider <cloud-provider> --binary-assets-path <path-to-binary-assets>
```
Note: 
1. Currently only AWS and GCP are supported for `provider` flag
1. The `binary-assets-path` is the path to the directory containing the binary assets for the recommender's internal kvcl. 
You can get the value for this field by running the following command:
    ```bash
    setup-envtest --os $(go env GOOS) --arch $(go env GOARCH) use $ENVTEST_K8S_VERSION -p path
    ```
    if the setup-envtest command is not available, you can install it by running the following command:
    ```bash
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    ```     

Once the scaling-recommender is launched, it is ready to receive http requests on `localhost:8080/recommend` endpoint. 

Below is an example piece of code which will fire an http request to the scaling-recommender:
    
```go
        reqURL := "http://localhost:8080/recommend/"
	reqBytes, err := json.Marshal(clusterSnapshot)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	r := bytes.NewReader(reqBytes)
	request, err := http.NewRequest(http.MethodPost, reqURL, r)

	if err != nil {
		fmt.Printf("Error creating request: %v", err)
		os.Exit(1)
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	response, err := client.Do(request)
```

