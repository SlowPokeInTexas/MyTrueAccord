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
)

type Debt struct {
	ID              int             `json:"id"`
	Amount          decimal.Decimal `json:"amount"`
	InPaymentPlan   bool            `json:"is_in_payment_plan,omitempty"`
	RemainingAmount decimal.Decimal `json:"remaining_amount,omitempty"`
	NextPaymentDate string          `json:"next_payment_due_date,omitempty"`
	paymentPlan     PaymentPlan
}

type PaymentPlan struct {
	ID                   int             `json:"id"`
	DebtID               int             `json:"debt_id"`
	AmountToPay          decimal.Decimal `json:"amount_to_pay"`
	InstallmentFrequency string          `json:"installment_frequency"`
	InstallmentAmount    decimal.Decimal `json:"installment_amount"`
	StartDate            string          `json:"start_date"`
	startDateValue       time.Time       `json:"start_date_value,omitempty"`
	payments             []Payment
}

type Payment struct {
	Amount        float64 `json:"amount"`
	Date          string  `json:"date"`
	PaymentPlanID int     `json:"payment_plan_id"`
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
	payments map[int]Payment
	err      error
}

func main() {
	var debtsChannel chan DebtsReturn = nil
	var paymentPlanChannel chan PaymentPlansReturn = nil
	var paymentsChannel chan PaymentsReturn = nil
	waitCount := 0 //  Could have used a waitgroup but I needed to grab return results

	debtsChannel = make(chan DebtsReturn)
	paymentPlanChannel = make(chan PaymentPlansReturn)
	paymentsChannel = make(chan PaymentsReturn)

	go pullDebts(debtsChannel, debtApiServer)
	go pullPaymentPlans(paymentPlanChannel, paymentPlanApiServer)
	go pullPayments(paymentsChannel, paymentsApiServer)

	var debts map[int]Debt
	var plans map[int]PaymentPlan
	var payments map[int]Payment

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
		fmt.Errorf("There was a problem gathering Debts, Payments, or Payment Plans.")
		return
	}

	//  Since all this ends up being hierarchical anyway, let's make it a graph
	err := unflattenData(debts, plans, payments)

	if err != nil {
		fmt.Errorf("Unexpected error encountered flattening data:%v", err)
		return
	}

	return
}

func unflattenData(debts map[int]Debt, paymentPlans map[int]PaymentPlan, payments map[int]Payment) error {
	var err error = nil
	for debtId, debt := range debts {

		//  Does this debt have an associated payment plan?
		plan, ok := paymentPlans[debtId]

		if ok {
			debt.paymentPlan = plan

			//  remove it from the map since we don't need it broken out anymore.
			//  Besides, we shall do some data integrity checking at the end to
			//  detect orphans
			delete(paymentPlans, debtId)

			//  Now attach the payments for that particular payment plan
			planId := debt.paymentPlan.ID

			//  We will use this slice to build up a list of payments that are relevant to a
			//  given payment plan
			tempPayments := make([]Payment, 1)

			//  We will use this slice to keep track of payments that have been added to a plan
			//  that we can remove from our payments collection
			paymentDeletions := make([]int, 1)

			//  Iterate through all the payments, matching the payments by plan id
			//  to their owner plans
			for pid, payment := range payments {
				if payment.PaymentPlanID == planId {
					tempPayments = append(tempPayments, payment)
				}
				paymentDeletions = append(paymentDeletions, pid)
			}
			//  Store those payments in the plan
			debt.paymentPlan.payments = tempPayments

			//  Now delete the loose payments that we've already associated
			//  with a payment plan
			for _, pid := range paymentDeletions {
				delete(payments, pid)
			}
		} // end if ok
	} //  end outer debt loop

	//  If we have any plans leftover, that's an error
	if len(paymentPlans) > 1 {
		//  in a production system these would show up in an exception report.
		err = fmt.Errorf("Found orphaned payment plans")
	}
	if len(payments) > 1 {
		//  ditto on the exception report here
		err = fmt.Errorf("Found orphaned payments")
	}
	return err
}

func pullPayments(results chan PaymentsReturn, serverUri string) {
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
	//  Now turn those into a map and return
	rvalue.payments = make(map[int]Payment)

	for _, pmt := range paymentsList {
		rvalue.payments[pmt.PaymentPlanID] = pmt
	}
	results <- rvalue
}

func pullDebts(results chan DebtsReturn, serverUri string) {
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

func pullPaymentPlans(results chan PaymentPlansReturn, serverUri string) {
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
		rvalue.paymentPlans[plan.DebtID] = plan
	}

	results <- rvalue
}

func transformDebtObjects(debts map[int]Debt) error {
	var err error = nil

	//  Iterate through each debt object, calculating

	return err
}

func IsPaymentPlanActive(debt *Debt, plan *PaymentPlan) bool {
	rc := false

	if debt != nil && plan != nil {

	}

	return rc
}
