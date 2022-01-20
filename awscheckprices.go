package awscheckprices

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/pricing"
)

type OptionTenancy string

var OptionTenancyShared OptionTenancy = "Shared"
var OptionTenancyHost OptionTenancy = "Host"
var OptionTenancyDedicated OptionTenancy = "Dedicated"

type OnDemandPrices []float64

type AvailabilityZone string

type SpotPrices map[AvailabilityZone][]float64

type priceCacheItem struct {
	age          time.Time
	instanceType string
	region       string
	tenancy      OptionTenancy
	p            *OnDemandPrices
	sp           *SpotPrices
}

type priceCacheItems struct {
	items []*priceCacheItem
	lock  *sync.RWMutex
}

var cache = func() *priceCacheItems {
	p := new(priceCacheItems)
	p.lock = new(sync.RWMutex)
	return p
}()

var CacheLifetime = time.Hour

func checkCache(instanceType string, region string, tenancy OptionTenancy, isSpot bool) *priceCacheItem {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	for _, i := range cache.items {
		if instanceType == i.instanceType && region == i.region && tenancy == i.tenancy && i.age.Add(CacheLifetime).After(time.Now()) {
			if (isSpot && i.sp != nil) || (!isSpot && i.p != nil) {
				return i
			}
		}
	}
	return nil
}

func checkCacheOnDemand(instanceType string, region string, tenancy OptionTenancy) OnDemandPrices {
	ret := checkCache(instanceType, region, tenancy, false)
	if ret == nil {
		return nil
	}
	p := *ret.p
	return p
}

func checkCacheSpot(instanceType string, region string, tenancy OptionTenancy) SpotPrices {
	ret := checkCache(instanceType, region, tenancy, true)
	if ret == nil {
		return nil
	}
	sp := *ret.sp
	return sp
}

func writeCache(instanceType string, region string, tenancy OptionTenancy, p *OnDemandPrices, sp *SpotPrices) {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	found := false
	for b, i := range cache.items {
		if instanceType == i.instanceType && region == i.region && tenancy == i.tenancy {
			found = true
			cache.items[b].age = time.Now()
			if p != nil {
				cache.items[b].p = p
			}
			if sp != nil {
				cache.items[b].sp = sp
			}
		}
	}
	if found {
		return
	}
	cache.items = append(cache.items, &priceCacheItem{
		age:          time.Now(),
		instanceType: instanceType,
		region:       region,
		tenancy:      tenancy,
		p:            p,
		sp:           sp,
	})
}

func GetPricesOnDemand(instanceType string, region string, tenancy OptionTenancy, session *session.Session) (p OnDemandPrices, err error) {
	p = checkCacheOnDemand(instanceType, region, tenancy)
	if p != nil {
		return p, err
	}
	svc := pricing.New(session, aws.NewConfig().WithRegion("us-east-1"))
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		MaxResults:  aws.Int64(100),
		Filters: []*pricing.Filter{
			&pricing.Filter{
				Field: aws.String("instanceType"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String(instanceType),
			},
			&pricing.Filter{
				Field: aws.String("regionCode"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String(region),
			},
			&pricing.Filter{
				Field: aws.String("marketoption"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("OnDemand"),
			},
			&pricing.Filter{
				Field: aws.String("tenancy"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String(string(tenancy)),
			},
			&pricing.Filter{
				Field: aws.String("capacitystatus"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("Used"), // other values: AllocatedCapacityReservation, UnusedCapacityReservation
			},
			&pricing.Filter{
				Field: aws.String("preInstalledSw"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("NA"),
			},
			&pricing.Filter{
				Field: aws.String("operatingSystem"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("Linux"),
			},
		},
	}

	result, err := svc.GetProducts(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case pricing.ErrCodeInternalErrorException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInternalErrorException, aerr.Error())
			case pricing.ErrCodeInvalidParameterException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidParameterException, aerr.Error())
			case pricing.ErrCodeNotFoundException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeNotFoundException, aerr.Error())
			case pricing.ErrCodeInvalidNextTokenException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidNextTokenException, aerr.Error())
			case pricing.ErrCodeExpiredNextTokenException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeExpiredNextTokenException, aerr.Error())
			default:
				return nil, fmt.Errorf("%s", aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			return nil, fmt.Errorf("%s", err.Error())
		}
	}
	p = OnDemandPrices{}
	for {
		for _, price := range result.PriceList {
			/*
				ret, err := json.MarshalIndent(price["terms"].(map[string]interface{})["OnDemand"].(map[string]interface{}), "", "    ")
				if err != nil {
					fmt.Println(err.Error())
				}
				fmt.Println(string(ret))
			*/
			for _, v := range price["terms"].(map[string]interface{})["OnDemand"].(map[string]interface{}) {
				for _, vv := range v.(map[string]interface{})["priceDimensions"].(map[string]interface{}) {
					if vv.(map[string]interface{})["unit"].(string) != "Hrs" {
						return nil, fmt.Errorf("price format incorrect:%s", vv.(map[string]interface{})["unit"].(string))
					}
					nPriceStr := vv.(map[string]interface{})["pricePerUnit"].(map[string]interface{})["USD"].(string)
					nPrice, err := strconv.ParseFloat(nPriceStr, 64)
					if err != nil {
						return nil, fmt.Errorf("price is NaN:%s: '%s'", err, nPriceStr)
					}
					p = append(p, nPrice)
				}
			}
		}
		input.NextToken = result.NextToken
		if input.NextToken == nil || *input.NextToken == "" {
			break
		}
		result, err = svc.GetProducts(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case pricing.ErrCodeInternalErrorException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInternalErrorException, aerr.Error())
				case pricing.ErrCodeInvalidParameterException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidParameterException, aerr.Error())
				case pricing.ErrCodeNotFoundException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeNotFoundException, aerr.Error())
				case pricing.ErrCodeInvalidNextTokenException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidNextTokenException, aerr.Error())
				case pricing.ErrCodeExpiredNextTokenException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeExpiredNextTokenException, aerr.Error())
				default:
					return nil, fmt.Errorf("%s", aerr.Error())
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				return nil, fmt.Errorf("%s", err.Error())
			}
		}
	}
	writeCache(instanceType, region, tenancy, &p, nil)
	return p, nil
}

func GetPricesSpot(region string, instanceType string, session *session.Session) (p SpotPrices, err error) {
	p = checkCacheSpot(instanceType, region, "")
	if p != nil {
		return p, err
	}
	utc, _ := time.LoadLocation("UTC")
	endTime := time.Now().In(utc)
	startTime := endTime.Add(-1 * time.Minute) // 1 minute ago
	input := &ec2.DescribeSpotPriceHistoryInput{
		StartTime:           &startTime,
		EndTime:             &endTime,
		MaxResults:          aws.Int64(100),
		ProductDescriptions: []*string{aws.String("Linux/UNIX")},
		InstanceTypes:       []*string{aws.String(instanceType)},
	}

	svc := ec2.New(session)
	result, err := svc.DescribeSpotPriceHistory(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				if !strings.Contains(aerr.Error(), "AuthFailure:") {
					return nil, fmt.Errorf("error 1: region: %s, %s", region, aerr.Error())
				}
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			return nil, fmt.Errorf("error 2: region: %s, %s", region, err.Error())
		}
	}
	p = make(SpotPrices)

	for {
		for _, price := range result.SpotPriceHistory {
			spotPrice, err := strconv.ParseFloat(*price.SpotPrice, 64)
			if err != nil {
				return nil, fmt.Errorf("could not parse float from %s: %s", *price.SpotPrice, err)
			}
			if _, ok := p[AvailabilityZone(*price.AvailabilityZone)]; !ok {
				p[AvailabilityZone(*price.AvailabilityZone)] = []float64{spotPrice}
			} else {
				p[AvailabilityZone(*price.AvailabilityZone)] = append(p[AvailabilityZone(*price.AvailabilityZone)], spotPrice)
			}
		}

		input.NextToken = result.NextToken
		if input.NextToken == nil || *input.NextToken == "" {
			break
		}
		result, err = svc.DescribeSpotPriceHistory(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				default:
					if !strings.Contains(aerr.Error(), "AuthFailure:") {
						return nil, fmt.Errorf("error 1: region: %s, %s", region, aerr.Error())
					}
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				return nil, fmt.Errorf("error 2: region: %s, %s", region, err.Error())
			}
		}
	}
	writeCache(instanceType, region, "", nil, &p)
	return p, nil
}
