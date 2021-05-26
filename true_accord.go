package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	debtApiServer        string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/debts"
	paymentPlanApiServer string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/payment_plans"
	paymentsApiServer    string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/payments"
	isoLayout            string = "2006-01-02"
	weekly               string = "weekly"
	biweekly             string = "bi_weekly"
	gracePeriodInDays    int    = 3
)

var (
	hoursInADay         time.Duration
	gracePeriodDuration time.Duration
)

func init() {
	hoursInADay, _ = time.ParseDuration("24h")
	gracePeriodDuration, _ = time.ParseDuration("120h")
	//  We want our decimals to be marshalled/unmarshalled without quotes, thank you very much
	decimal.MarshalJSONWithoutQuotes = true
}

type Debt struct {
	ID                        int             `json:"id"`
	Amount                    decimal.Decimal `json:"amount"`
	InPaymentPlan             bool            `json:"is_in_payment_plan"`
	RemainingAmount           decimal.Decimal `json:"remaining_amount"`
	remainingAmountCalculated bool
	NextPaymentDate           *string `json:"next_payment_due_date"`
	paymentPlan               *PaymentPlan
}

type PaymentPlan struct {
	ID                   int             `json:"id"`
	DebtID               int             `json:"debt_id"`
	AmountToPay          decimal.Decimal `json:"amount_to_pay"`
	InstallmentFrequency string          `json:"installment_frequency"`
	InstallmentAmount    decimal.Decimal `json:"installment_amount"`
	StartDate            string          `json:"start_date"`
	startDate            time.Time       //  The date converted to golang date format
	payments             []Payment
}

type Payment struct {
	Amount        decimal.Decimal `json:"amount"`
	Date          string          `json:"date"`
	date          time.Time       //  The date converted to golang date format
	PaymentPlanID int             `json:"payment_plan_id"`
}

//  Used to grab results and error codes from the goroutine which
//  retrieves Debts from the web-service
type DebtsReturn struct {
	debts map[int]Debt
	err   error
}

//  Used to grab results and error codes from the goroutine which
//  retrieves PaymentPlans from the web-service
type PaymentPlansReturn struct {
	paymentPlans map[int]PaymentPlan
	err          error
}

//  Used to grab results and error codes from the goroutine which
//  retrieves Payments from the web-service
type PaymentsReturn struct {
	payments []Payment
	err      error
}

func main() {
	var err error = nil

	var debts map[int]Debt

	var debtList []Debt

	//  Populate the debts structure which includes debts, plans and payments
	err = populateDebtHierarchy(&debts)

	if err != nil {
		fmt.Printf("Error populating debts:%v", err)
		return
	}

	debtList = make([]Debt, len(debts))

	idx := 0

	for _, debt := range debts {
		debtList[idx] = debt
		idx++
	}

	bytes, tempError := json.MarshalIndent(debtList, "", "   ")

	if tempError != nil {
		fmt.Printf("Error marshalling output:%v", tempError)
	} else {
		fmt.Printf("%v\n", string(bytes))
	}

	return
}

//  return a graph of the data returned from the service calls.
//  I'm aware of the memory implications of this, but the
//  the services operations are currently designed (specifically, we get the
//  entirety of a result-set with each call, rather than being able
//  to load by id or specify a subset), we are left with two choices:
//  1. Make multiple cascading "retrieve all" calls to the services for each debt,
//  payment. This would quickly saturate the service infrastructure with any sort
//  of volume in production and generally would be quite gnarly.
//  2. Cache all our entries locally in memory.
//  Obviously, we chose option 2
func populateDebtHierarchy(debts *map[int]Debt) error {
	var err error = nil

	var debtsChannel chan DebtsReturn = nil
	var paymentPlanChannel chan PaymentPlansReturn = nil
	var paymentsChannel chan PaymentsReturn = nil
	waitCount := 0 //  Could have used a waitgroup but I needed to grab return results

	debtsChannel = make(chan DebtsReturn)
	paymentPlanChannel = make(chan PaymentPlansReturn)
	paymentsChannel = make(chan PaymentsReturn)

	go retrieveDebts(debtsChannel, debtApiServer)
	go retrievePaymentPlans(paymentPlanChannel, paymentPlanApiServer)
	go retrievePayments(paymentsChannel, paymentsApiServer)

	var plans map[int]PaymentPlan
	var payments []Payment

	//  I didn't use a waitgroup here because I need a timeout
	for timedOut := false; waitCount < 3 && timedOut != true; {
		select {
		case debtWrapper := <-debtsChannel:
			waitCount++
			if debtWrapper.err == nil {
				*debts = debtWrapper.debts
			} else {
				fmt.Errorf("Error encountered retrieving or parsing Debts:%v", debtWrapper.err)
			}
			if waitCount > 2 {
				break
			}
		case planWrapper := <-paymentPlanChannel:
			waitCount++
			if planWrapper.err == nil {
				plans = planWrapper.paymentPlans
			} else {
				fmt.Errorf("Error encountered retrieving or parsing Payment Plans:%v", planWrapper.err)
			}

			if waitCount > 2 {
				break
			}
		case paymentsWrapper := <-paymentsChannel:
			waitCount++
			if paymentsWrapper.err == nil {
				payments = paymentsWrapper.payments
			} else {
				fmt.Errorf("Error encountered retrieving or parsing Payment Plans:%v", paymentsWrapper.err)
			}

			if waitCount > 2 {
				break
			}

		case <-time.After(240 * time.Second):
			fmt.Println("Timed out waiting for one or more results")
			timedOut = true
			break
		}
	}

	if plans == nil || debts == nil || payments == nil {
		return fmt.Errorf("There was a problem gathering Debts, Payments, or Payment Plans.")
	}

	//  Since all this ends up being hierarchical anyway, let's make it a graph
	err = normalizeData(*debts, plans, payments)

	if err != nil {
		return fmt.Errorf("Unexpected error encountered flattening data:%v", err)
	}

	return err
}

func normalizeData(debts map[int]Debt, paymentPlans map[int]PaymentPlan, payments []Payment) error {
	var err error = nil
	for debtId, debt := range debts {

		//  Does this debt have an associated payment plan?
		plan, ok := paymentPlans[debtId]

		if ok {
			debt.paymentPlan = &plan

			//  remove it from the map since we don't need it broken out anymore.
			//  Besides, we shall do some data integrity checking at the end to
			//  detect orphans
			delete(paymentPlans, debtId)

			//  Now attach the payments for that particular payment plan
			planId := debt.paymentPlan.ID

			//  We will use this slice to build up a list of payments that are relevant to a
			//  given payment plan
			var tempPayments []Payment

			//  Iterate through all the payments, matching the payments by plan id
			//  to their owner plans
			for _, pmt := range payments {
				if pmt.PaymentPlanID == planId {
					tempPayments = append(tempPayments, pmt)
				}
			}
			//  Store those payments in the plan
			debt.paymentPlan.payments = tempPayments

			//
			debt.calculateNextPaymentDate(true)

			debt.InPaymentPlan = debt.isPaymentPlanActive()

		} // end if ok
		//  Store the modified debt object back in the collection
		debts[debtId] = debt
	} //  end outer debt loop

	//  If we have any plans leftover, that's an error
	if len(paymentPlans) > 1 {
		//  in a production system these would show up in an exception report.
		err = fmt.Errorf("Found orphaned payment plans")
	}

	return err
}

func retrievePayments(results chan PaymentsReturn, serverUri string) {
	var rvalue PaymentsReturn
	var err error = nil
	var resp *http.Response = nil

	if len(serverUri) < 1 {
		rvalue.err = fmt.Errorf("Invalid Server URI passed")
		results <- rvalue
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", serverUri, nil)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Connection", "keep-alive")

	resp, err = client.Do(req)
	if err != nil || resp == nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	if resp.StatusCode != 200 {
		rvalue.err = fmt.Errorf("Unexpected Status Code:%v", resp.StatusCode)
		results <- rvalue
		return
	}

	defer func() {
		if resp != nil {
			if resp.Body != nil {
				_ = resp.Body.Close()
				resp.Body = nil
			}
			resp = nil
		}
		client.CloseIdleConnections()
		client = nil
	}()

	var bytes []byte = nil

	bytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}

	//  Pull out the list of debts first as an array
	var paymentsList []Payment
	err = json.Unmarshal(bytes, &paymentsList)

	//  Make sure the json parsed okay
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}

	for _, pmt := range paymentsList {
		if len(pmt.Date) > 0 {
			pmt.date, err = time.Parse(isoLayout, pmt.Date)

			if err != nil {
				rvalue.err = err
				results <- rvalue
				return
			}
		}
		rvalue.payments = append(rvalue.payments, pmt)

	}
	//  Sort the payments by date to make our lives easier later
	sort.Slice(rvalue.payments, func(i, j int) bool { return rvalue.payments[i].date.Before(rvalue.payments[j].date) })
	results <- rvalue
}

func retrieveDebts(results chan DebtsReturn, serverUri string) {
	var rvalue DebtsReturn
	var err error = nil
	var resp *http.Response = nil

	if len(serverUri) < 1 {
		rvalue.err = fmt.Errorf("Invalid Server URI passed")
		results <- rvalue
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", serverUri, nil)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Connection", "keep-alive")

	resp, err = client.Do(req)
	if err != nil || resp == nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	if resp.StatusCode != 200 {
		rvalue.err = fmt.Errorf("Unexpected Status Code:%v", resp.StatusCode)
		results <- rvalue
		return
	}

	defer func() {
		if resp != nil {
			if resp.Body != nil {
				_ = resp.Body.Close()
				resp.Body = nil
			}
			resp = nil
		}
		client.CloseIdleConnections()
		client = nil
	}()

	var bytes []byte = nil

	bytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}

	//  Pull out the list of debts first as an array
	var debtList []Debt
	err = json.Unmarshal(bytes, &debtList)

	//  Make sure the json parsed okay
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	//  Now turn those into a map and return
	rvalue.debts = make(map[int]Debt)

	for _, debt := range debtList {
		rvalue.debts[debt.ID] = debt
	}

	results <- rvalue
}

func retrievePaymentPlans(results chan PaymentPlansReturn, serverUri string) {
	var rvalue PaymentPlansReturn
	var err error = nil
	var resp *http.Response = nil

	if len(serverUri) < 1 {
		rvalue.err = fmt.Errorf("Invalid Server URI passed")
		results <- rvalue
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", serverUri, nil)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Connection", "keep-alive")

	resp, err = client.Do(req)
	if err != nil || resp == nil {
		rvalue.err = err
		results <- rvalue
		return
	}
	if resp.StatusCode != 200 {
		rvalue.err = fmt.Errorf("Unexpected Status Code:%v", resp.StatusCode)
		results <- rvalue
		return
	}

	defer func() {
		if resp != nil {
			if resp.Body != nil {
				resp.Body.Close()
				resp.Body = nil
			}
			resp = nil
		}
		client.CloseIdleConnections()
		client = nil
	}()

	var bytes []byte = nil

	bytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}

	//  Pull out the list of payment plans first as an array
	var paymentPlans []PaymentPlan
	err = json.Unmarshal(bytes, &paymentPlans)

	//  Make sure the json parsed okay
	if err != nil {
		rvalue.err = err
		results <- rvalue
		return
	}

	//  Now turn those into a map and return
	rvalue.paymentPlans = make(map[int]PaymentPlan)

	//  Use the Debt-id as a key since we're going to have to perform lookups based on that
	for _, plan := range paymentPlans {

		//  While we're at it, parse the dates..
		if len(plan.StartDate) > 0 {
			plan.startDate, err = time.Parse(isoLayout, plan.StartDate)

			if err != nil {
				rvalue.err = err
				results <- rvalue
				return
			}
		}

		if err == nil {
			rvalue.paymentPlans[plan.DebtID] = plan
		}
	}

	results <- rvalue
}

func (debt *Debt) sumTotalPayments() (decimal.Decimal, int) {
	var rvalue decimal.Decimal
	var paymentCount int

	if debt.paymentPlan != nil {

		plan := debt.paymentPlan

		if plan.payments != nil {
			for _, payment := range plan.payments {
				paymentCount++
				rvalue = rvalue.Add(payment.Amount).Round(2)
			}
		}
	}

	return rvalue, paymentCount
}

func (debt *Debt) isDebtPaidOff() bool {
	rc := false
	if !debt.remainingAmountCalculated {
		debt.CalculateRemainingAmount(true)
	}
	//  Check for zero or negative. It's possible they over-paid
	if debt.RemainingAmount.IsZero() || debt.RemainingAmount.IsNegative() {
		rc = true
	}
	return rc
}

func (debt *Debt) CalculateRemainingAmount(updateObject bool) decimal.Decimal {
	var rvalue decimal.Decimal

	//  See how much has been paid, if anything
	amountPaid, _ := debt.sumTotalPayments()

	//  Start by the setting to the debt's amount
	rvalue = debt.Amount

	//  If there's a payment plan, use the amount_to_pay from there
	if debt.paymentPlan != nil {
		if !debt.paymentPlan.AmountToPay.Equal(debt.Amount) {
			rvalue = debt.paymentPlan.AmountToPay
		}
	}

	//  Now set the remaining amount on the object
	rvalue = rvalue.Sub(amountPaid).Round(2)
	if updateObject {
		debt.remainingAmountCalculated = true
		debt.RemainingAmount = rvalue
	}
	return rvalue
}

func (debt *Debt) isPaymentPlanActive() bool {
	rc := false

	if debt.paymentPlan != nil {
		if !debt.isDebtPaidOff() {
			rc = true
		}
	}
	return rc
}

//  We have a potentially nasty set of problems related to matching up scheduled payment
//  dates with actual dates and amounts:
//  1. Given the data provided, we are missing the original scheduled payoff date, so we
//      can't work backwards from that.
//  2. Given the data provided, we are also missing the original debt amount, which
//      in conjunction with remaining_amount, we might have used to infer a payment schedule
//  3. As mentioned in the spec, the customer could have made random payments that are not on schedule
//  4. Valid scheduled payments may not have posted on their scheduled dates
//  Given those problems, we're going to make a couple of assumptions:
//  i. A payment that falls within a certain number of days of a payment date will count
//      as a scheduled payment. We are going to call that a grace period, and is defined
//      as the constant 'gracePeriodInDays' at the top of a program.
func (debt *Debt) calculateNextPaymentDate(updateObject bool) string {
	var nextPaymentDate string

	//  First make sure a payment plan is active
	if debt.isPaymentPlanActive() {
		//  Does this debt have any outstanding payments?
		paymentCount := len(debt.paymentPlan.payments)
		if paymentCount > 0 {
			var nextScheduledDate time.Time

			var xpayment Payment

			//  Starting with most recent payment made and working backwards,
			//  Grab the last payment that was made and then add the payment period
			for i := paymentCount - 1; i >= 0 && nextScheduledDate.IsZero(); i-- {
				pmt := debt.paymentPlan.payments[i]

				lastScheduledPaymentDate, err := debt.lastScheduledDateNotExceedingPaymentDate(pmt.date)

				//  If one of those failed, I don't know what to tell ya'...
				if err != nil || lastScheduledPaymentDate.IsZero() {
					return nextPaymentDate
				} else {
					d, tempErr := paymentFrequencyAsDuration(debt.paymentPlan.InstallmentFrequency)

					if tempErr != nil {
						return nextPaymentDate
					}

					//  Add the duration to the schedule
					nextScheduledDate = lastScheduledPaymentDate.Add(d)
					xpayment = pmt
				}
			}

			if !nextScheduledDate.IsZero() {
				nextPaymentDate = nextScheduledDate.String()

				if !nextScheduledDate.After(xpayment.date) {
					fmt.Printf("Last Payment date (%v) is after the chosen next date (%v)", xpayment.date, nextScheduledDate)
				}
			}
		}
	}

	if updateObject && len(nextPaymentDate) > 0 {
		debt.NextPaymentDate = &nextPaymentDate
	}

	return nextPaymentDate
}
func paymentFrequencyAsDuration(freq string) (time.Duration, error) {
	var rvalue time.Duration
	var err error = nil
	var frequency string
	var dayAddValue int

	//  Normalize our comparison to lowercase; you never know what those Scala services are up to 😁
	frequency = strings.ToLower(freq)

	switch frequency {
	case weekly:
		dayAddValue = 7
		break

	case biweekly:
		dayAddValue = 14
		break
	default:
		//  punt if we got something unexpected
		return rvalue, fmt.Errorf("Received unexpected value of %v in payment frequency", freq)
	}

	rvalue, err = time.ParseDuration(fmt.Sprintf("%vh", 24*dayAddValue))

	return rvalue, err
}

func (debt *Debt) lastScheduledDateNotExceedingPaymentDate(date time.Time) (time.Time, error) {
	var rvalue time.Time
	var err error = nil

	if debt.isPaymentPlanActive() {
		var current time.Time
		var last time.Time

		var frequencyDuration time.Duration

		frequencyDuration, err = paymentFrequencyAsDuration(debt.paymentPlan.InstallmentFrequency)

		current = debt.paymentPlan.startDate
		last = current

		for current.Before(date) {
			//  Add days to hit the next payment cycle point
			current = current.Add(frequencyDuration)

			//  if the current date is after the payment date, break...
			if current.After(date) || current == date {
				break
			}
			last = current
		}

		if current == date {
			rvalue = current
		} else {
			rvalue = last
		}
	}

	return rvalue, err
}

func datesWithinGracePeriodRange(t1 time.Time, t2 time.Time) bool {
	rc := false

	d := t1.Sub(t2)

	if math.Abs(float64(d)) <= float64(gracePeriodDuration) {
		rc = true
	}
	return rc
}
