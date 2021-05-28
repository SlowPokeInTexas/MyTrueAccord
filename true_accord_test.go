package main

import (
	"testing"
	"time"
	_ "unicode"

	"github.com/shopspring/decimal"
)

func makeMockGraph(debts *map[int]Debt) error {
	var plans map[int]PaymentPlan
	var payments []Payment
	var err error = nil

	*debts, plans, payments = getRawTestObjects()

	err = normalizeData(*debts, plans, payments)

	return err
}
func getRawTestObjects() (debtTestData map[int]Debt, paymentPlanTestData map[int]PaymentPlan, paymentsTestData []Payment) {
	debtTestData = map[int]Debt{
		0:  Debt{Amount: decimal.NewFromFloat(1500000.00), ID: 0},
		1:  Debt{Amount: decimal.NewFromFloat(1234.00), ID: 1},
		2:  Debt{Amount: decimal.NewFromFloat(50000), ID: 2},
		3:  Debt{Amount: decimal.NewFromFloat(400), ID: 3},
		4:  Debt{Amount: decimal.NewFromFloat(123.46), ID: 4},
		5:  Debt{Amount: decimal.NewFromFloat(100), ID: 5},
		6:  Debt{Amount: decimal.NewFromFloat(4920.34), ID: 6},
		7:  Debt{Amount: decimal.NewFromFloat(12938), ID: 7},
		8:  Debt{Amount: decimal.NewFromFloat(9238.02), ID: 8},
		9:  Debt{Amount: decimal.NewFromFloat(0.0), ID: 9},
		10: Debt{Amount: decimal.NewFromFloat(10000), ID: 10}, //  Testing debt with no payment plan
		11: Debt{Amount: decimal.NewFromFloat(5281), ID: 11},  //  Testing for a payment that started before the plan
	}

	paymentPlanTestData = map[int]PaymentPlan{
		0:  {ID: 0, DebtID: 0, AmountToPay: decimal.NewFromFloat(1000000.00), InstallmentFrequency: "bi_weekly", InstallmentAmount: decimal.NewFromInt32(1000), StartDate: "2021-05-31"}, //  Test Payments scheduled in the future
		1:  {ID: 1, DebtID: 1, AmountToPay: decimal.NewFromFloat(0.00), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt32(175), StartDate: "2020-01-31"},
		2:  {ID: 2, DebtID: 2, AmountToPay: decimal.NewFromFloat(42000.00), InstallmentFrequency: "bi_weekly", InstallmentAmount: decimal.NewFromInt32(300), StartDate: "2020-05-28"},
		3:  {ID: 3, DebtID: 3, AmountToPay: decimal.NewFromFloat(399.00), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt32(25), StartDate: "2020-10-21"},
		4:  {ID: 4, DebtID: 4, AmountToPay: decimal.NewFromFloat(123.46), InstallmentFrequency: "bi_weekly", InstallmentAmount: decimal.NewFromFloat(5.28), StartDate: "2020-02-28"},
		5:  {ID: 5, DebtID: 5, AmountToPay: decimal.NewFromInt(75), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt32(5.00), StartDate: "2020-03-12"},
		6:  {ID: 6, DebtID: 6, AmountToPay: decimal.NewFromInt(4500.00), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt32(100.00), StartDate: "2020-08-12"},
		7:  {ID: 7, DebtID: 7, AmountToPay: decimal.NewFromInt(12500.00), InstallmentFrequency: "bi_weekly", InstallmentAmount: decimal.NewFromInt32(250.00), StartDate: "2020-02-05"},
		8:  {ID: 8, DebtID: 8, AmountToPay: decimal.NewFromInt(90000.00), InstallmentFrequency: "bi_weekly", InstallmentAmount: decimal.NewFromInt32(250.00), StartDate: "2020-02-05"},
		9:  {ID: 9, DebtID: 9, AmountToPay: decimal.NewFromInt(0.00), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt32(250.00), StartDate: "2020-02-05"},
		11: {ID: 11, DebtID: 11, AmountToPay: decimal.NewFromInt(5281), InstallmentFrequency: "weekly", InstallmentAmount: decimal.NewFromInt(25), StartDate: "2020-11-05"},
	}

	paymentsTestData = []Payment{
		{PaymentPlanID: 1, Amount: decimal.NewFromFloat(50.00), Date: "2021-05-15"},

		{PaymentPlanID: 2, Amount: decimal.NewFromInt(725), Date: "2020-06-02"},
		{PaymentPlanID: 2, Amount: decimal.NewFromInt(1000), Date: "2020-06-02"},       //  Try two payments on the same unscheduled date
		{PaymentPlanID: 2, Amount: decimal.NewFromFloat(1000.36), Date: "2020-06-28"},  //  Folow-up with two payments on scheduled date
		{PaymentPlanID: 2, Amount: decimal.NewFromFloat(1500.77), Date: "2020-06-28"},  //  Folow-up with two payments on schedule date
		{PaymentPlanID: 2, Amount: decimal.NewFromFloat(1500.55), Date: "2020-06-29"},  //  Folow-up with two payments on schedule date
		{PaymentPlanID: 2, Amount: decimal.NewFromFloat(10000.71), Date: "2021-04-01"}, //  Wait several months then make whopping payment

		{PaymentPlanID: 3, Amount: decimal.NewFromFloat(25), Date: "2020-11-03"},
		{PaymentPlanID: 3, Amount: decimal.NewFromFloat(30), Date: "2020-11-17"},
		{PaymentPlanID: 3, Amount: decimal.NewFromFloat(25), Date: "2020-12-01"},
		{PaymentPlanID: 3, Amount: decimal.NewFromFloat(65), Date: "2021-01-01"},

		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-03-14"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-03-28"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-03-14"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-04-11"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-04-25"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-05-09"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-05-23"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-06-06"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-06-20"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-07-04"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-07-18"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-08-01"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-08-15"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-08-29"},
		{PaymentPlanID: 4, Amount: decimal.NewFromFloat(5.28), Date: "2020-09-12"},

		{PaymentPlanID: 9, Amount: decimal.NewFromFloat(100.00), Date: "2020-09-12"},

		{PaymentPlanID: 11, Amount: decimal.NewFromFloat(125.00), Date: "2020-08-31"},
	}

	for key, plan := range paymentPlanTestData {
		if len(plan.StartDate) > 0 {
			plan.startDate, _ = time.Parse(isoLayout, plan.StartDate)
			paymentPlanTestData[key] = plan
		}
	}

	for idx, pmt := range paymentsTestData {
		if len(pmt.Date) > 0 {
			pmt.date, _ = time.Parse(isoLayout, pmt.Date)
			paymentsTestData[idx] = pmt
		}
	}

	return debtTestData, paymentPlanTestData, paymentsTestData
}

func TestDebt_calculateRemainingAmount(t *testing.T) {
	var debts map[int]Debt
	var err error = nil
	var got decimal.Decimal
	var want decimal.Decimal

	err = makeMockGraph(&debts)

	if err != nil {
		t.Errorf("calculateRemainingAmount(), error making mock data: %v", err)
		return
	}

	//  Check to make sure we pick up amount from paymentplan rather than debt
	t.Logf("Checking that we fall back to PaymentPlan for a payment amount")
	debt := debts[0]
	got = debt.calculateRemainingAmount(false)
	want, err = decimal.NewFromString("1000000")
	if err != nil {
		t.Errorf("calculateRemainingAmount(), error converting decimal from string mock data: %v", err)
	}
	if !want.Equal(got) {
		t.Errorf("calculateRemainingAmount(), want:%v, got:%v", want, got)
	}

	//  Check a paymentplan that has a zero in it
	//  Check to make sure we pick up amount from paymentplan rather than debt
	t.Logf("Checking a paymentplan that has a zero in 'amount_to_pay'")
	debt = debts[1]
	got = debt.calculateRemainingAmount(false)
	want, err = decimal.NewFromString("1184")
	if err != nil {
		t.Errorf("calculateRemainingAmount(), error converting decimal from string mock data: %v", err)
	}
	if !want.Equal(got) {
		t.Errorf("calculateRemainingAmount(), want:%v, got:%v", want, got)
	}

	//  Check a bunch of payments
	t.Logf("Checking a bunch of payments")
	debt = debts[4]
	got = debt.calculateRemainingAmount(false)
	want, err = decimal.NewFromString("44.26")
	if err != nil {
		t.Errorf("calculateRemainingAmount(), error converting decimal from string mock data: %v", err)
	}
	if !want.Equal(got) {
		t.Errorf("calculateRemainingAmount(), want:%v, got:%v", want, got)
	}

	//  Check a debt that should be paid off but there was an extra payment
	t.Logf("Check a debt that should be paid off but there was an extra payment")
	debt = debts[9]
	got = debt.calculateRemainingAmount(false)
	want, err = decimal.NewFromString("-100.00")
	if err != nil {
		t.Errorf("calculateRemainingAmount(), error converting decimal from string mock data: %v", err)
	}
	if !want.Equal(got) {
		t.Errorf("calculateRemainingAmount(), want:%v, got:%v", want, got)
	}

	//  Check debt with no payment plan
	t.Logf("Check debt with no payment plan")
	debt = debts[10]
	got = debt.calculateRemainingAmount(false)
	want, err = decimal.NewFromString("10000.00")
	if err != nil {
		t.Errorf("calculateRemainingAmount(), error converting decimal from string mock data: %v", err)
	}
	if !want.Equal(got) {
		t.Errorf("calculateRemainingAmount(), want:%v, got:%v", want, got)
	}
}

func TestDebt_calculateNextPaymentDate(t *testing.T) {
	var debts map[int]Debt
	var err error = nil
	var got time.Time
	var want time.Time
	var dateString string

	err = makeMockGraph(&debts)
	if err != nil {
		t.Errorf("calculateNextPaymentDate(), error making mock data: %v", err)
		return
	}

	//  Test for date that occurs before plan begins. This should never happen,
	//  but lots of things should never happen but do.
	t.Logf("Checking the next scheduled date when payments occur before the start date")
	debt := debts[11]
	dateString = debt.calculateNextPaymentDate(false)
	got, err = time.Parse(isoLayout, dateString)
	if err != nil {
		t.Errorf("calculateNextPaymentDate() error parsing date returned from calculateNextPaymentDate (%v):%v", dateString, err)
	}
	want, err = time.Parse(isoLayout, "2020-11-12")
	if got != want {
		t.Errorf("Got:%v but wanted %v", got, want)
	}

	t.Logf("Trying to confuse the next-date algorithm")
	debt = debts[2]
	dateString = debt.calculateNextPaymentDate(false)
	got, err = time.Parse(isoLayout, dateString)
	if err != nil {
		t.Errorf("calculateNextPaymentDate() error parsing date returned from calculateNextPaymentDate (%v):%v", dateString, err)
	}
	want, err = time.Parse(isoLayout, "2021-04-15")
	if got != want {
		t.Errorf("Got:%v but wanted %v", got, want)
	}

	t.Logf("Checking no payments made on correct date")
	debt = debts[3]
	dateString = debt.calculateNextPaymentDate(false)
	got, err = time.Parse(isoLayout, dateString)
	if err != nil {
		t.Errorf("calculateNextPaymentDate() error parsing date returned from calculateNextPaymentDate (%v):%v", dateString, err)
	}
	want, err = time.Parse(isoLayout, "2021-01-06")
	if got != want {
		t.Errorf("Got:%v but wanted %v", got, want)
	}
}

func TestDebt_isDebtPaidOff(t *testing.T) {
	var debts map[int]Debt
	var err error = nil
	var got bool
	var want bool

	err = makeMockGraph(&debts)
	if err != nil {
		t.Errorf("calculateNextPaymentDate(), error making mock data: %v", err)
		return
	}

	//  Try a debt that should be paid off
	t.Logf("Checking a debt that should be paid off")
	debt := debts[9]
	got = debt.isDebtPaidOff()
	want = true
	if got != want {
		t.Errorf("Testing isDebtPaidOff  Got:%v, Want:%v", got, want)
	}

	//  Try a debt that shouldn't be paid off
	t.Logf("Checking a debt that should NOT be paid off")
	debt = debts[6]
	got = debt.isDebtPaidOff()
	want = false
	if got != want {
		t.Errorf("Testing isDebtPaidOff  Got:%v, Want:%v", got, want)
	}
}
