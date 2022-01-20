# aws-check-prices

## Specs

```go
// GetPricesOnDemand - function to query on-demand prices
func GetPricesOnDemand(instanceType string, region string, tenancy OptionTenancy, session *session.Session) (p OnDemandPrices, err error)

// GetPricesSpot - function to query spot instance prices
func GetPricesSpot(region string, instanceType string, session *session.Session) (p SpotPrices, err error)

// OptionTenancy type
type OptionTenancy string

// OptionTenancy types - to be specified for on-demand instance price listings
var OptionTenancyShared OptionTenancy = "Shared"       // Shared tenancy
var OptionTenancyHost OptionTenancy = "Host"           // host tenancy
var OptionTenancyDedicated OptionTenancy = "Dedicated" // dedicated tenancy

// OnDemandPrices - list of prices returned by AWS for the on-demand query
type OnDemandPrices []float64

// SpotPrices - a list of spot prices returned for spot instances, per availability zone
type SpotPrices map[AvailabilityZone][]float64

// AvailabilityZobe type
type AvailabilityZone string

// CacheLifetime to be specified as how long the cache should remain alive
var CacheLifetime time.Duration
```

## Usage

### Specify lifetime of cache

```go
awscheckprices.CacheLifetime = time.Hour
```

### Connect to AWS

```go
session, err := session.NewSession()
if err != nil {
	log.Println(err)
}
```

### Get prices for on-demand instances

```go
prices, err := awscheckprices.GetPricesOnDemand("t3a.medium", "eu-west-1", awscheckprices.OptionTenancyShared, session)
if err != nil {
	log.Fatal(err)
}
log.Println(prices)
```

### Get prices for spot instances

```go
spotPrices, err := awscheckprices.GetPricesSpot("eu-west-1", "t3a.medium", session)
if err != nil {
	log.Fatal(err)
}
log.Println(spotPrices)
```

## Full Example

```go
package main

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	awscheckprices "github.com/rglonek/aws-check-prices"
)

func main() {
	awscheckprices.CacheLifetime = time.Hour

	session, err := session.NewSession()
	if err != nil {
		log.Println(err)
	}

	prices, err := awscheckprices.GetPricesOnDemand("t3a.medium", "eu-west-1", awscheckprices.OptionTenancyShared, session)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(prices)

	spotPrices, err := awscheckprices.GetPricesSpot("eu-west-1", "t3a.medium", session)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(spotPrices)
}
```
