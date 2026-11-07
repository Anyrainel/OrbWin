package lanhelper

import "errors"

const nativeRuleRecordBudget = 8 << 20
const nativeRuleRecordEvents = 16

type nativeRuleWriteState struct {
	phase         string
	events, bytes int
	failed        bool
}

func (state *nativeRuleWriteState) append(kind string, data []byte, write func([]byte) (int, error), flush func() error) error {
	if state.failed || state.events >= nativeRuleRecordEvents || len(data) == 0 || len(data) > 256<<10 || state.bytes+len(data)+1 > nativeRuleRecordBudget {
		return errors.New("protected native rule journal poisoned or over budget")
	}
	want := map[string]string{"": "create-intent", "create-intent": "create-returned", "create-returned": "created", "created": "remove-intent", "remove-intent": "removed"}[state.phase]
	if kind != want {
		return errors.New("protected native rule event order differs; no replay")
	}
	data = append(append([]byte(nil), data...), '\n')
	state.failed = true
	n, err := write(data)
	if err != nil || n != len(data) {
		return errors.New("protected native rule write unknown")
	}
	if err = flush(); err != nil {
		return errors.New("protected native rule flush unknown")
	}
	state.phase = kind
	state.events++
	state.bytes += len(data)
	state.failed = false
	return nil
}
