package starlark

import (
	"testing"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/testutil"
	"github.com/stretchr/testify/require"
)

// Tests for runtime errors in the processors Init function.
func TestInitError(t *testing.T) {
	tests := []struct {
		name   string
		plugin *Starlark
	}{
		{
			name: "source must define apply",
			plugin: &Starlark{
				Source:  "",
				OnError: "drop",
				Log:     testutil.Logger{},
			},
		},
		{
			name: "apply must be a function",
			plugin: &Starlark{
				Source: `
apply = 42
`,
				OnError: "drop",
				Log:     testutil.Logger{},
			},
		},
		{
			name: "apply function must take one arg",
			plugin: &Starlark{
				Source: `
def apply():
	pass
`,
				OnError: "drop",
				Log:     testutil.Logger{},
			},
		},
		{
			name: "package scope must have valid syntax",
			plugin: &Starlark{
				Source: `
for
`,
				OnError: "drop",
				Log:     testutil.Logger{},
			},
		},
		{
			name: "on_error must have valid choice",
			plugin: &Starlark{
				Source: `
def apply(metric):
	pass
`,
				OnError: "foo",
				Log:     testutil.Logger{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Init()
			require.Error(t, err)
		})
	}
}

func TestApply(t *testing.T) {
	// Tests for the behavior of the processors Apply function.
	var applyTests = []struct {
		name     string
		source   string
		input    []telegraf.Metric
		expected []telegraf.Metric
	}{
		{
			name: "drop metric",
			source: `
def apply(metric):
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "passthrough",
			source: `
def apply(metric):
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "read value from global scope",
			source: `
names = {
	'cpu': 'cpu2',
	'mem': 'mem2',
}

def apply(metric):
	metric.name = names[metric.name]
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu2",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "cannot write to frozen global scope",
			source: `
cache = []

def apply(metric):
	cache.append(deepcopy(metric))
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 1.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "cannot return multiple references to same metric",
			source: `
def apply(metric):
	# Should be return [metric, deepcopy(metric)]
	return [metric, metric]
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
	}

	for _, tt := range applyTests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &Starlark{
				Source:  tt.source,
				OnError: "drop",
				Log:     testutil.Logger{},
			}
			err := plugin.Init()
			require.NoError(t, err)

			actual := plugin.Apply(tt.input...)
			testutil.RequireMetricsEqual(t, tt.expected, actual)
		})
	}
}

// Tests for the behavior of the Metric type.
func TestMetric(t *testing.T) {
	var tests = []struct {
		name     string
		source   string
		input    []telegraf.Metric
		expected []telegraf.Metric
	}{
		{
			name: "create new metric",
			source: `
def apply(metric):
	m = Metric('cpu')
	m.fields['time_guest'] = 2.0
	m.time = 0
	return m
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest": 2.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "deepcopy",
			source: `
def apply(metric):
	return [metric, deepcopy(metric)]
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set name",
			source: `
def apply(metric):
	metric.name = "howdy"
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("howdy",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set name wrong type",
			source: `
def apply(metric):
	metric.name = 42
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "get name",
			source: `
def apply(metric):
	metric.tags['measurement'] = metric.name
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"measurement": "cpu",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "getattr tags",
			source: `
def apply(metric):
	metric.tags
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "setattr tags is not allowed",
			source: `
def apply(metric):
	metric.tags = {}
	return metric
		`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "empty tags are false",
			source: `
def apply(metric):
	if not metric.tags:
		return metric
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "non-empty tags are true",
			source: `
def apply(metric):
	if metric.tags:
		return metric
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags in operator",
			source: `
def apply(metric):
	if 'host' not in metric.tags:
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup tag",
			source: `
def apply(metric):
	metric.tags['result'] = metric.tags['host']
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host":   "example.org",
						"result": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup tag not set",
			source: `
def apply(metric):
	metric.tags['foo']
	return metric
		`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "set tag",
			source: `
def apply(metric):
	metric.tags['host'] = 'example.org'
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set tag type error",
			source: `
def apply(metric):
	metric.tags['host'] = 42
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "pop tag",
			source: `
def apply(metric):
	metric.tags['host2'] = metric.tags.pop('host')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host2": "example.org",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "popitem tags",
			source: `
def apply(metric):
	metric.tags['result'] = '='.join(metric.tags.popitem())
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"result": "host=example.org",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "popitem tags empty dict",
			source: `
def apply(metric):
	metric.tags.popitem()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "tags setdefault key not set",
			source: `
def apply(metric):
	metric.tags.setdefault('a', 'b')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags setdefault key already set",
			source: `
def apply(metric):
	metric.tags.setdefault('a', 'c')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags update list of tuple",
			source: `
def apply(metric):
	metric.tags.update([('b', 'y'), ('c', 'z')])
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
						"b": "y",
						"c": "z",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags update kwargs",
			source: `
def apply(metric):
	metric.tags.update(b='y', c='z')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
						"b": "y",
						"c": "z",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags update dict",
			source: `
def apply(metric):
	metric.tags.update({'b': 'y', 'c': 'z'})
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
						"b": "y",
						"c": "z",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags update list tuple and kwargs",
			source: `
def apply(metric):
	metric.tags.update([('b', 'y'), ('c', 'z')], d='zz')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "x",
						"b": "y",
						"c": "z",
						"d": "zz",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tags",
			source: `
def apply(metric):
	for k in metric.tags:
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
						"foo":  "bar",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
						"foo":  "bar",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tags and copy to fields",
			source: `
def apply(metric):
	for k in metric.tags:
		metric.fields[k] = k
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{
						"host":      "host",
						"cpu":       "cpu",
						"time_idle": 42,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag keys",
			source: `
def apply(metric):
	for k in metric.tags.keys():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
						"foo":  "bar",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
						"foo":  "bar",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag keys and copy to fields",
			source: `
def apply(metric):
	for k in metric.tags.keys():
		metric.fields[k] = k
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{
						"host":      "host",
						"cpu":       "cpu",
						"time_idle": 42,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag items",
			source: `
def apply(metric):
	for k, v in metric.tags.items():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag items and copy to fields",
			source: `
def apply(metric):
	for k, v in metric.tags.items():
		metric.fields[k] = v
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{
						"time_idle": 42,
						"host":      "example.org",
						"cpu":       "cpu0",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag values",
			source: `
def apply(metric):
	for v in metric.tags.values():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag values and copy to fields",
			source: `
def apply(metric):
	for v in metric.tags.values():
		metric.fields[v] = v
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
						"cpu":  "cpu0",
					},
					map[string]interface{}{
						"time_idle":   42,
						"example.org": "example.org",
						"cpu0":        "cpu0",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "clear tags",
			source: `
def apply(metric):
	metric.tags.clear()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tags cannot pop while iterating",
			source: `
def apply(metric):
	for k in metric.tags:
		metric.tags.pop(k)
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "tags cannot popitem while iterating",
			source: `
def apply(metric):
	for k in metric.tags:
		metric.tags.popitem()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "tags cannot clear while iterating",
			source: `
def apply(metric):
	for k in metric.tags:
		metric.tags.clear()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "tags cannot insert while iterating",
			source: `
def apply(metric):
	for k in metric.tags:
		metric.tags['i'] = 'j'
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "getattr fields",
			source: `
def apply(metric):
	metric.fields
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "setattr fields is not allowed",
			source: `
def apply(metric):
	metric.fields = {}
	return metric
		`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "empty fields are false",
			source: `
def apply(metric):
	if not metric.fields:
		return metric
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "non-empty fields are true",
			source: `
def apply(metric):
	if metric.fields:
		return metric
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields in operator",
			source: `
def apply(metric):
	if 'time_idle' not in metric.fields:
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup string field",
			source: `
def apply(metric):
	value = metric.fields['value']
	if value != "xyzzy" and type(value) != "str":
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": "xyzzy"},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": "xyzzy"},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup integer field",
			source: `
def apply(metric):
	value = metric.fields['value']
	if value != 42 and type(value) != "int":
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup unsigned field",
			source: `
def apply(metric):
	value = metric.fields['value']
	if value != 42 and type(value) != "int":
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": uint64(42)},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": uint64(42)},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup bool field",
			source: `
def apply(metric):
	value = metric.fields['value']
	if value != True and type(value) != "bool":
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": true},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": true},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup float field",
			source: `
def apply(metric):
	value = metric.fields['value']
	if value != 42.0 and type(value) != "float":
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": 42.0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"value": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "lookup field not set",
			source: `
def apply(metric):
	metric.fields['foo']
	return metric
		`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "set string field",
			source: `
def apply(metric):
	metric.fields['host'] = 'example.org'
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"host": "example.org",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set integer field",
			source: `
def apply(metric):
	metric.fields['time_idle'] = 42
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set float field",
			source: `
def apply(metric):
	metric.fields['time_idle'] = 42.0
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set bool field",
			source: `
def apply(metric):
	metric.fields['time_idle'] = True
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": true,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set field type error",
			source: `
def apply(metric):
	metric.fields['time_idle'] = {}
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "pop field",
			source: `
def apply(metric):
	time_idle = metric.fields.pop('time_idle')
	if time_idle != 0:
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "popitem field",
			source: `
def apply(metric):
	item = metric.fields.popitem()
	if item != ("time_idle", 0):
		return
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 0},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "popitem fields empty dict",
			source: `
def apply(metric):
	metric.fields.popitem()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "fields setdefault key not set",
			source: `
def apply(metric):
	metric.fields.setdefault('a', 'b')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"a": "b"},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields setdefault key already set",
			source: `
def apply(metric):
	metric.fields.setdefault('a', 'c')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"a": "b"},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"a": "b"},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields update list of tuple",
			source: `
def apply(metric):
	metric.fields.update([('a', 'b'), ('c', 'd')])
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields update kwargs",
			source: `
def apply(metric):
	metric.fields.update(a='b', c='d')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields update dict",
			source: `
def apply(metric):
	metric.fields.update({'a': 'b', 'c': 'd'})
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "fields update list tuple and kwargs",
			source: `
def apply(metric):
	metric.fields.update([('a', 'b'), ('c', 'd')], e='f')
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate fields",
			source: `
def apply(metric):
	for k in metric.fields:
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field keys",
			source: `
def apply(metric):
	for k in metric.fields.keys():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field keys and copy to tags",
			source: `
def apply(metric):
	for k in metric.fields.keys():
		metric.tags[k] = k
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"time_guest":  "time_guest",
						"time_idle":   "time_idle",
						"time_system": "time_system",
					},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field items",
			source: `
def apply(metric):
	for k, v in metric.fields.items():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.0,
						"time_idle":   2.0,
						"time_system": 3.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field items and copy to tags",
			source: `
def apply(metric):
	for k, v in metric.fields.items():
		metric.tags[k] = str(v)
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_guest":  1.1,
						"time_idle":   2.1,
						"time_system": 3.1,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"time_guest":  "1.1",
						"time_idle":   "2.1",
						"time_system": "3.1",
					},
					map[string]interface{}{
						"time_guest":  1.1,
						"time_idle":   2.1,
						"time_system": 3.1,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field values",
			source: `
def apply(metric):
	for v in metric.fields.values():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field values and copy to tags",
			source: `
def apply(metric):
	for v in metric.fields.values():
		metric.tags[str(v)] = str(v)
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"b": "b",
						"d": "d",
						"f": "f",
					},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "clear fields",
			source: `
def apply(metric):
	metric.fields.clear()
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle":   0,
						"time_guest":  0,
						"time_system": 0,
						"time_user":   0,
					},
					time.Unix(0, 0),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set time",
			source: `
def apply(metric):
	metric.time = 42
	return metric
			`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(0, 0).UTC(),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(0, 42).UTC(),
				),
			},
		},
		{
			name: "set time wrong type",
			source: `
def apply(metric):
	metric.time = 'howdy'
	return metric
			`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(0, 0).UTC(),
				),
			},
			expected: []telegraf.Metric{},
		},
		{
			name: "get time",
			source: `
def apply(metric):
	metric.time -= metric.time % 100000000
	return metric
			`,
			input: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(42, 11).UTC(),
				),
			},
			expected: []telegraf.Metric{
				testutil.MustMetric(
					"cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42,
					},
					time.Unix(42, 0).UTC(),
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &Starlark{
				Source:  tt.source,
				OnError: "drop",
				Log:     testutil.Logger{},
			}
			err := plugin.Init()
			require.NoError(t, err)

			actual := plugin.Apply(tt.input...)
			testutil.RequireMetricsEqual(t, tt.expected, actual)
		})
	}
}

// Benchmarks modify the metric in place, so the scripts shouldn't modify the
// metric.
func Benchmark(b *testing.B) {
	var tests = []struct {
		name   string
		source string
		input  []telegraf.Metric
	}{
		{
			name: "passthrough",
			source: `
def apply(metric):
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "create new metric",
			source: `
def apply(metric):
	m = Metric('cpu')
	m.fields['time_guest'] = 2.0
	m.time = 0
	return m
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set name",
			source: `
def apply(metric):
	metric.name = "cpu"
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set tag",
			source: `
def apply(metric):
	metric.tags['host'] = 'example.org'
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"host": "example.org",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "tag in operator",
			source: `
def apply(metric):
	if 'c' in metric.tags:
		return metric
	return None
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tags",
			source: `
def apply(metric):
	for k in metric.tags:
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 42.0},
					time.Unix(0, 0),
				),
			},
		},
		{
			// This should be faster than calling items()
			name: "iterate tags and get values",
			source: `
def apply(metric):
	for k in metric.tags:
		v = metric.tags[k]
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate tag items",
			source: `
def apply(metric):
	for k, v in metric.tags.items():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					map[string]interface{}{"time_idle": 42},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "set string field",
			source: `
def apply(metric):
	metric.fields['host'] = 'example.org'
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"host": "example.org",
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate fields",
			source: `
def apply(metric):
	for k in metric.fields:
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle":   42.0,
						"time_user":   42.0,
						"time_guest":  42.0,
						"time_system": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			// This should be faster than calling items()
			name: "iterate fields and get values",
			source: `
def apply(metric):
	for k in metric.fields:
		v = metric.fields[k]
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"time_idle":   42.0,
						"time_user":   42.0,
						"time_guest":  42.0,
						"time_system": 42.0,
					},
					time.Unix(0, 0),
				),
			},
		},
		{
			name: "iterate field items",
			source: `
def apply(metric):
	for k, v in metric.fields.items():
		pass
	return metric
`,
			input: []telegraf.Metric{
				testutil.MustMetric("cpu",
					map[string]string{},
					map[string]interface{}{
						"a": "b",
						"c": "d",
						"e": "f",
						"g": "h",
					},
					time.Unix(0, 0),
				),
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			plugin := &Starlark{
				Source:  tt.source,
				OnError: "drop",
				Log:     testutil.Logger{},
			}

			err := plugin.Init()
			require.NoError(b, err)

			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				plugin.Apply(tt.input...)
			}
		})
	}
}
