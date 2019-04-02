package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type QueryData struct {
	// The user that you need to interact with.
	Username string

	// This is indicated by "data-buyvalue".
	YouPay float64

	// This is indicated by "data-sellvalue".
	YouReceive float64

	// The amount the user has in stock.
	Stock int64
}

// This type is just used as a key to cache previous queries.
type QueryArgs struct {
	Have int64
	Want int64
}

type PathNode struct {
	// The ID of the currency in this hop.
	Id int64

	// The amount of this currency received for the previous currency/amount in the path.
	Amount int64

	// The user who will give you this deal.
	User string
}

var (
	id2Currency = map[int64]string{
		1:  "alteration",
		2:  "fusing",
		3:  "alchemy",
		4:  "chaos",
		6:  "exalted",
		7:  "chromatic",
		8:  "jewellers",
		9:  "chance",
		10: "chisel",
		11: "scouring",
		12: "blessed",
		13: "regret",
		14: "regal",
		16: "vaal",
		17: "wisdom",
		18: "portal",
		19: "armorer",
		20: "whetstone",
		21: "glassblower",
		22: "alteration",
		23: "chance",
		35: "silver",
		27: "sacrifice_at_dusk",
		28: "sacrifice_at_midnight",
		29: "sacrifice_at_dawn",
		30: "sacrifice_at_noon",
	}

	// The start of the line in the response that contains the QueryData we want.
	grepForData = "<div class=\"displayoffer \" data-username="

	// The currency website.
	website = "currency.poe.trade"

	league = flag.String("league", "Synthesis", "The POE league to use")

	// If a page has already been queried, the results are cached in this map.
	pageCache = map[QueryArgs]QueryData{}

	// Max depth for the DFS. Keep it >2.
	maxDepthDFS = flag.Int("dfs_depth", 3, "The max depth for the DFS. Keep it >2")

	// Starting currency.
	startingCurrency = flag.String("currency", "chaos", "The starting currency for this scheme")

	// Starting currency amount.
	startingAmount = flag.Int64("amount", 10, "The starting currency amount")

	// This accumulates all of the successful cycles.
	pathsToRiches = make([][]PathNode, 0, *maxDepthDFS+1)
)

func parseLine(line string) QueryData {
	//  Line is of the format:
	//             <div class="displayoffer " data-username="GOrdonFlicker" data-sellcurrency="1" data-sellvalue="4.0" data-buycurrency="2" data-buyvalue="1.0" data-ign="Rangeroided" data-stock="45">

	// TODO: Make this less disgusting.
	splitLine := strings.Split(line, "=")

	tmp := splitLine[2]
	username := strings.Split(tmp, "\"")[1]

	tmp = splitLine[4]
	youReceive, err := strconv.ParseFloat(strings.Split(tmp, "\"")[1], 64)
	if err != nil {
		log.Fatal(err)
	}

	tmp = splitLine[6]
	youPay, err := strconv.ParseFloat(strings.Split(tmp, "\"")[1], 64)
	if err != nil {
		log.Fatal(err)
	}

	data := QueryData{
		Username:   username,
		YouReceive: youReceive,
		YouPay:     youPay,
		Stock:      -1,
	}
	return data
}

// Returns the parsed market data for the best deal and whether there were any deals to be made.
func scrapePage(site string, haveCurrency int64, wantCurrency int64, haveAmount int64) (QueryData, bool) {
	// Check if this is cached already.
	args := QueryArgs{
		Have: haveCurrency,
		Want: wantCurrency,
	}
	if data, ok := pageCache[args]; ok {
		log.Printf("Cache hit for wantCurrency=%s, haveCurrency=%s", wantCurrency, haveCurrency)
		return data, true
	}

	query := fmt.Sprintf("http://%s/search?league=%s&online=x&stock=&want=%d&have=%d", website, *league, wantCurrency, haveCurrency)
	log.Printf("GETing endpoint %s", query)

	response, err := http.Get(query)
	if err != nil {
		log.Fatal(err)
	}
	if response.StatusCode != 200 {
		log.Fatalf("Bad response: %+v", *response)
	}
	defer response.Body.Close()

	// Filter out the relevant line.
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		if strings.Contains(line, grepForData) &&
			strings.Contains(line, "data-sellcurrency") &&
			strings.Contains(line, "data-sellvalue") &&
			strings.Contains(line, "data-buycurrency") &&
			strings.Contains(line, "data-buyvalue") &&
			strings.Contains(line, "data-ign") {

			tmp := parseLine(line)
			// This only works if we have enough currency to use this trade.
			if tmp.YouPay <= float64(haveAmount) {
				pageCache[args] = tmp
				return tmp, true
			}
		}
	}

	return QueryData{}, false
}

func UpdateContext(context []PathNode, currencyId int64, queryData QueryData) []PathNode {
	lastNode := context[len(context)-1]
	n := PathNode{
		Id:     currencyId,
		Amount: int64(float64(lastNode.Amount) / queryData.YouPay * queryData.YouReceive),
		User:   queryData.Username,
	}
	log.Printf("Appending to trade path with %+v", n)
	return append(context, n)
}

func PopContext(context []PathNode) []PathNode {
	lastNode := context[len(context)-1]
	log.Printf("Popping from trade path: %+v", lastNode)
	if len(context) == 0 {
		log.Fatal("attempting to pop empty context")
	}
	return context[:len(context)-1]
}

// Perform the DFS. Returns true if we found a valid cycle.
func Delve(currencyId int64, context []PathNode) bool {
	// Have we come back to where we started?
	if len(context) > 0 && context[0].Id == currencyId {
		log.Print("====== back where we started ======")
		log.Printf("@tallen: %+v", context)
		tmp := make([]PathNode, len(context))
		copy(tmp, context)
		pathsToRiches = append(pathsToRiches, tmp)
		return true
	}

	// Have we delved too deep?
	if len(context) > *maxDepthDFS {
		log.Fatal("this is a bug- delved too deep.")
	} else if len(context) == *maxDepthDFS {
		log.Printf("Hit max depth:")
		return false
	}

	// Iterate through all currencies.
	for tradePartnerCurrencyId, currencyName := range id2Currency {
		if tradePartnerCurrencyId == currencyId {
			// No reason to exchange currency with itself.
			continue
		}
		log.Printf("Investigating  %s -> %s", id2Currency[currencyId], currencyName)

		if len(context) == 0 {
			// Populate the starting node.
			n := PathNode{
				Id:     currencyId,
				Amount: *startingAmount,
				User:   "N/A",
			}
			log.Printf("Populating starting node: %+v", n)
			context = append(context, n)
		}

		// Scrape data to see what trade looks like.
		lastNode := context[len(context)-1]
		queryData, valid := scrapePage(website, currencyId, tradePartnerCurrencyId, lastNode.Amount)
		if !valid {
			return false
		}

		// We've received a valid trade in 'queryData' at this point. If the trade path is empty, we'll
		// need to add the first hop as a special case, then add the trade in 'queryData'.

		context = UpdateContext(context, tradePartnerCurrencyId, queryData)
		Delve(tradePartnerCurrencyId, context)
		context = PopContext(context)
	}

	return true
}

func Report(riches [][]PathNode) {
	for _, c := range riches {
		fmt.Println("---------")
		for n, path := range c {
			fmt.Printf("Step %d: %s @ %d\n", n, id2Currency[path.Id], path.Amount)
		}
	}
}

func main() {
	for id, currencyName := range id2Currency {
		if *startingCurrency == currencyName {
			log.Printf("Starting with currency=%s and amount=%d", currencyName, *startingAmount)
			c := make([]PathNode, 0, *maxDepthDFS+1)
			Delve(id, c)
			log.Printf("Finished!")
			Report(pathsToRiches)
			return
		}
	}

	fmt.Println("Looks like nothing ran. Here's a summary of valid currencies:")
	for _, c := range id2Currency {
		fmt.Println(c)
	}
}
