# aws-check-prices

## Example

```go
package main

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
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
