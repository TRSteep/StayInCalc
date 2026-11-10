package main

import (
	"strings"
	"testing"
)

func TestKazakhstanTwoWeekEntryShowsFullStayAndConditionalReset(t *testing.T) {
	rule := russianRules(t)["KZ"]
	trips := []Trip{{Entry: "2026-08-05", Exit: "2026-11-02"}}
	departure, ref := d("2026-11-02"), d("2026-11-10")
	plan := planReturn(trips, rule, departure, ref, 14, false)
	if plan == nil || plan.Entry != d("2027-02-01") || plan.OutsideDays != 90 {
		t.Fatalf("entry: %+v", plan)
	}
	end, limited := returnStayEnd(trips, rule, departure, plan)
	if limited || end != d("2027-05-01") {
		t.Fatalf("maximum stay: %s, limited %v", fd(end), limited)
	}
	var b strings.Builder
	r := evaluate(trips, rule.Limits, resetGap(rule), ref, rule)
	returnPlans(&b, trips, rule, r, nil)
	for _, text := range []string{
		"## Возвращение", "# Ближайший возможный въезд на 2 недели: 01.02.2027",
		"# Пребывание возможно до: 01.05.2027", "# Полное обнуление дней: 01.05.2027",
		"если до неё оставаться вне страны", "нулевого счётчика в эту дату уже не будет",
	} {
		if !strings.Contains(b.String(), text) {
			t.Fatalf("missing %q: %s", text, b.String())
		}
	}
}

func TestReturnStayEndKeepsRegistrationAndEntryDayRules(t *testing.T) {
	rules := russianRules(t)
	for _, tc := range []struct{ country, want string }{{"UZ", "2026-10-15"}, {"CN", "2026-10-31"}} {
		plan := planReturn(nil, rules[tc.country], d("2026-10-01"), d("2026-10-01"), 14, false)
		if plan == nil {
			t.Fatal(tc.country)
		}
		end, limited := returnStayEnd(nil, rules[tc.country], d("2026-10-01"), plan)
		if limited || end != d(tc.want) {
			t.Fatalf("%s: %s, limited %v", tc.country, fd(end), limited)
		}
	}
	plan := &ReturnPlan{Entry: d("2026-10-01"), Registered: true}
	end, limited := returnStayEnd(nil, rules["UZ"], d("2026-10-01"), plan)
	if !end.IsZero() || !limited {
		t.Fatal("registered Uzbekistan must not invent an unlimited exit date")
	}
}
