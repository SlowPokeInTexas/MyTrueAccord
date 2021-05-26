package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

const (
	debtApiServer        string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/debts"
	paymentPlanApiServer string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/payment_plans"
	paymentsApiServer    string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/payments"
	isoLayout            string = "2006-01-02"
)

type Debt struct {
	ID                        int             `json:"id"`
	Amount                    decimal.Decimal `json:"amount"`
	InPaymentPlan             bool            `json:"is_in_payment_plan,omitempty"`
	RemainingAmount           decimal.Decimal `json:"remaining_amount,omitempty"`
	remainingAmountCalculated bool
	NextPaymentDate           string `json:"next_payment_due_date,omitempty"`
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

	var debts map[int]Debt

	//  Populate the debts structure which includes debts, plans and payments
	err := populateDebtHierarchy(debts)

	if err != nil {
		fmt.Printf("Error populating debts:%v", err)
		return
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
func populateDebtHierarchy(debts map[int]Debt) error {
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
				debts = debtWrapper.debts
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
	err = unflattenData(debts, plans, payments)

	if err != nil {
		return fmt.Errorf("Unexpected error encountered flattening data:%v", err)
	}

	return err
}

func unflattenData(debts map[int]Debt, paymentPlans map[int]PaymentPlan, payments []Payment) error {
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

func (debt *Debt) sumTotalPayments() decimal.Decimal {
	var rvalue decimal.Decimal
	if debt.paymentPlan != nil {

		plan := debt.paymentPlan

		if plan.payments != nil {
			for _, payment := range plan.payments {
				rvalue.Add(payment.Amount)
			}
		}
	}

	return rvalue
}

func (debt *Debt) IsPaidOff() bool {
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
	amountPaid := debt.sumTotalPayments()

	//  Start by the setting to the debt's amount
	rvalue = debt.Amount

	//  If there's a payment plan, use the amount_to_pay from there
	if debt.paymentPlan != nil {
		rvalue = debt.paymentPlan.AmountToPay
	}

	//  Now set the remaining amount on the object
	rvalue = rvalue.Sub(amountPaid)

	if updateObject {
		debt.remainingAmountCalculated = true
		debt.RemainingAmount = rvalue
	}
	return rvalue
}

func (debt *Debt) isPaymentPlanActive() bool {
	rc := false

	if debt.paymentPlan != nil {

		if !debt.IsPaidOff() {
			rc = true
		}
	}
	return rc
}
