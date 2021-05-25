package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	debtApiServer    string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/debts"
	paymentApiServer string = "https://my-json-server.typicode.com/druska/trueaccord-mock-payments-api/payment_plans"
)

type Debt struct {
	ID            int     `json:"id"`
	Amount        float64 `json:"amount"`
	InPaymentPlan bool    `json:"is_in_payment_plan,omitempty"`
}

type PaymentPlan struct {
	ID                   int     `json:"id"`
	DebtID               int     `json:"debt_id"`
	AmountToPay          float64 `json:"amount_to_pay"`
	InstallmentFrequency string  `json:"installment_frequency"`
	InstallmentAmount    float64 `json:"installment_amount"`
	StartDate            string  `json:"start_date"`
}

//  Used to grab results and error codes from a goroutine
type DebtsReturn struct {
	debts map[int]Debt
	err   error
}

//  Used to grab results and error codes from a goroutine
type PaymentPlansReturn struct {
	paymentPlans map[int]PaymentPlan
	err          error
}

func main() {
	var debtsChannel chan DebtsReturn = nil
	var paymentsChannel chan PaymentPlansReturn = nil
	waitCount := 0 //  Could have used a waitgroup but I needed to grab return results

	debtsChannel = make(chan DebtsReturn)
	paymentsChannel = make(chan PaymentPlansReturn)

	go pullDebts(debtsChannel, debtApiServer)
	go pullPayments(paymentsChannel, paymentApiServer)

	var debts map[int]Debt
	var plans map[int]PaymentPlan

	for timedOut := false; waitCount < 2 && timedOut != true; {
		select {
		case debtWrapper := <-debtsChannel:
			waitCount++
			if debtWrapper.err == nil {
				debts = debtWrapper.debts
			} else {
				fmt.Errorf("Error encountered retrieving or parsing Debts:%v", debtWrapper.err)
			}
			if waitCount > 1 {
				break
			}
		case planWrapper := <-paymentsChannel:
			waitCount++
			if planWrapper.err == nil {
				plans = planWrapper.paymentPlans
			} else {
				fmt.Errorf("Error encountered retrieving or parsing Payment Plans:%v", planWrapper.err)
			}

			if waitCount > 1 {
				break
			}
		case <-time.After(240 * time.Second):
			fmt.Println("Timed out")
			timedOut = true
			break
		}
	}

	if plans == nil || debts == nil {
		fmt.Errorf("There was a problem gathering Debts or Payment Plans. kthxbye.")
		return
	}

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

func pullPayments(results chan PaymentPlansReturn, serverUri string) {
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

	for _, plan := range paymentPlans {
		rvalue.paymentPlans[plan.ID] = plan
	}

	results <- rvalue
}
