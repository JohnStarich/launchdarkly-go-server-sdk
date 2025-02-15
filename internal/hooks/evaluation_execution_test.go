package hooks

import (
	"context"
	"errors"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

func emptyExecutionAssertions(t *testing.T, res *EvaluationExecution, ldContext ldcontext.Context) {
	assert.Empty(t, res.hooks)
	assert.Empty(t, res.data)
	assert.Equal(t, ldContext, res.context.Context())
	assert.Equal(t, "test-flag", res.context.FlagKey())
	assert.Equal(t, "testMethod", res.context.Method())
	assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())
}

type orderTracker struct {
	orderBefore []string
	orderAfter  []string
}

func newOrderTracker() *orderTracker {
	return &orderTracker{
		orderBefore: make([]string, 0),
		orderAfter:  make([]string, 0),
	}
}

func TestEvaluationExecution(t *testing.T) {
	falseValue := ldvalue.Bool(false)
	ldContext := ldcontext.New("test-context")

	t.Run("with no hooks", func(t *testing.T) {
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{})

		t.Run("run before evaluation", func(t *testing.T) {
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
				"testMethod")
			execution.BeforeEvaluation(context.Background())
			emptyExecutionAssertions(t, execution, ldContext)
		})

		t.Run("run after evaluation", func(t *testing.T) {
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
				"testMethod")
			execution.AfterEvaluation(context.Background(),
				ldreason.NewEvaluationDetail(falseValue, 0,
					ldreason.NewEvalReasonFallthrough()))
			emptyExecutionAssertions(t, execution, ldContext)
		})
	})

	t.Run("with hooks", func(t *testing.T) {
		t.Run("prepare evaluation series", func(t *testing.T) {
			hookA := sharedtest.NewTestHook("a")
			hookB := sharedtest.NewTestHook("b")
			runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

			ldContext := ldcontext.New("test-context")
			res := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue, "testMethod")

			assert.Len(t, res.hooks, 2)
			assert.Len(t, res.data, 2)
			assert.Equal(t, ldContext, res.context.Context())
			assert.Equal(t, "test-flag", res.context.FlagKey())
			assert.Equal(t, "testMethod", res.context.Method())
			assert.Equal(t, falseValue, res.context.DefaultValue())
			assert.Equal(t, res.data[0], ldhooks.EmptyEvaluationSeriesData())
			assert.Equal(t, res.data[1], ldhooks.EmptyEvaluationSeriesData())
		})

		t.Run("verify execution order", func(t *testing.T) {
			testCases := []struct {
				name                string
				method              func(execution *EvaluationExecution)
				expectedBeforeOrder []string
				expectedAfterOrder  []string
			}{
				{name: "BeforeEvaluation",
					method: func(execution *EvaluationExecution) {
						execution.BeforeEvaluation(context.Background())
					},
					expectedBeforeOrder: []string{"a", "b"},
					expectedAfterOrder:  make([]string, 0),
				},
				{name: "AfterEvaluation",
					method: func(execution *EvaluationExecution) {
						detail := ldreason.NewEvaluationDetail(falseValue, 0,
							ldreason.NewEvalReasonFallthrough())
						execution.AfterEvaluation(context.Background(), detail)
					},
					expectedBeforeOrder: make([]string, 0),
					expectedAfterOrder:  []string{"b", "a"}},
			}

			t.Run("with hooks registered at config time", func(t *testing.T) {
				for _, testCase := range testCases {
					t.Run(testCase.name, func(t *testing.T) {
						tracker := newOrderTracker()
						hookA := createOrderTrackingHook("a", tracker)
						hookB := createOrderTrackingHook("b", tracker)
						runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

						execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
							"testMethod")
						testCase.method(execution)

						// BeforeEvaluation should execute in registration order.
						assert.Equal(t, testCase.expectedBeforeOrder, tracker.orderBefore)
						assert.Equal(t, testCase.expectedAfterOrder, tracker.orderAfter)
					})
				}
			})

			t.Run("run before evaluation", func(t *testing.T) {
				hookA := sharedtest.NewTestHook("a")
				hookB := sharedtest.NewTestHook("b")
				runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

				execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
					"testMethod")
				execution.BeforeEvaluation(context.Background())

				hookA.Verify(t, sharedtest.HookExpectedCall{
					HookStage: sharedtest.HookStageBeforeEvaluation,
					EvalCapture: sharedtest.HookEvalCapture{
						EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
							falseValue, "testMethod"),
						EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
						GoContext:            context.Background(),
					}})

				hookB.Verify(t, sharedtest.HookExpectedCall{
					HookStage: sharedtest.HookStageBeforeEvaluation,
					EvalCapture: sharedtest.HookEvalCapture{
						EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
							falseValue, "testMethod"),
						EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
						GoContext:            context.Background(),
					}})
			})

			t.Run("run after evaluation", func(t *testing.T) {
				hookA := sharedtest.NewTestHook("a")
				hookB := sharedtest.NewTestHook("b")
				runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

				execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
					"testMethod")
				detail := ldreason.NewEvaluationDetail(falseValue, 0,
					ldreason.NewEvalReasonFallthrough())
				execution.AfterEvaluation(context.Background(), detail)

				hookA.Verify(t, sharedtest.HookExpectedCall{
					HookStage: sharedtest.HookStageAfterEvaluation,
					EvalCapture: sharedtest.HookEvalCapture{
						EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
							falseValue, "testMethod"),
						EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
						Detail:               detail,
						GoContext:            context.Background(),
					}})

				hookB.Verify(t, sharedtest.HookExpectedCall{
					HookStage: sharedtest.HookStageAfterEvaluation,
					EvalCapture: sharedtest.HookEvalCapture{
						EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
							falseValue, "testMethod"),
						EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
						Detail:               detail,
						GoContext:            context.Background(),
					}})
			})

			t.Run("run before evaluation with an error", func(t *testing.T) {
				mockLog := ldlogtest.NewMockLog()
				hookA := sharedtest.NewTestHook("a")
				hookA.BeforeInject = func(
					ctx context.Context,
					seriesContext ldhooks.EvaluationSeriesContext,
					data ldhooks.EvaluationSeriesData,
				) (ldhooks.EvaluationSeriesData, error) {
					return ldhooks.NewEvaluationSeriesBuilder(data).
						Set("testA", "A").
						Build(), errors.New("something bad")
				}
				hookB := sharedtest.NewTestHook("b")
				hookB.BeforeInject = func(
					ctx context.Context,
					seriesContext ldhooks.EvaluationSeriesContext,
					data ldhooks.EvaluationSeriesData,
				) (ldhooks.EvaluationSeriesData, error) {
					return ldhooks.NewEvaluationSeriesBuilder(data).
						Set("testB", "testB").
						Build(), nil
				}

				runner := NewRunner(mockLog.Loggers, []ldhooks.Hook{hookA, hookB})
				execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
					"testMethod")

				execution.BeforeEvaluation(context.Background())
				assert.Len(t, execution.hooks, 2)
				assert.Len(t, execution.data, 2)
				assert.Equal(t, ldContext, execution.context.Context())
				assert.Equal(t, "test-flag", execution.context.FlagKey())
				assert.Equal(t, "testMethod", execution.context.Method())
				assert.Equal(t, ldhooks.EmptyEvaluationSeriesData(), execution.data[0])
				assert.Equal(t,
					ldhooks.NewEvaluationSeriesBuilder(
						ldhooks.EmptyEvaluationSeriesData()).
						Set("testB", "testB").
						Build(), execution.data[1])
				assert.Equal(t, falseValue, execution.context.DefaultValue())

				assert.Equal(t, []string{"During evaluation of flag \"test-flag\", an error was encountered in \"BeforeEvaluation\" of the \"a\" hook: something bad"},
					mockLog.GetOutput(ldlog.Error))
			})

			t.Run("run after evaluation with an error", func(t *testing.T) {
				mockLog := ldlogtest.NewMockLog()
				hookA := sharedtest.NewTestHook("a")
				// The hooks execute in reverse order, so we have an error in B and check that A still executes.
				hookA.AfterInject = func(
					ctx context.Context,
					seriesContext ldhooks.EvaluationSeriesContext,
					data ldhooks.EvaluationSeriesData,
					detail ldreason.EvaluationDetail,
				) (ldhooks.EvaluationSeriesData, error) {
					return ldhooks.NewEvaluationSeriesBuilder(data).
						Set("testA", "testA").
						Build(), nil
				}
				hookB := sharedtest.NewTestHook("b")
				hookB.AfterInject = func(
					ctx context.Context,
					seriesContext ldhooks.EvaluationSeriesContext,
					data ldhooks.EvaluationSeriesData,
					detail ldreason.EvaluationDetail,
				) (ldhooks.EvaluationSeriesData, error) {
					return ldhooks.NewEvaluationSeriesBuilder(data).
						Set("testB", "B").
						Build(), errors.New("something bad")

				}

				runner := NewRunner(mockLog.Loggers, []ldhooks.Hook{hookA, hookB})
				execution := runner.prepareEvaluationSeries("test-flag", ldContext, falseValue,
					"testMethod")
				detail := ldreason.NewEvaluationDetail(falseValue, 0,
					ldreason.NewEvalReasonFallthrough())

				execution.AfterEvaluation(context.Background(), detail)
				assert.Len(t, execution.hooks, 2)
				assert.Len(t, execution.data, 2)
				assert.Equal(t, ldContext, execution.context.Context())
				assert.Equal(t, "test-flag", execution.context.FlagKey())
				assert.Equal(t, "testMethod", execution.context.Method())
				assert.Equal(t, ldhooks.EmptyEvaluationSeriesData(), execution.data[1])
				assert.Equal(t,
					ldhooks.NewEvaluationSeriesBuilder(
						ldhooks.EmptyEvaluationSeriesData()).
						Set("testA", "testA").
						Build(), execution.data[0])
				assert.Equal(t, falseValue, execution.context.DefaultValue())
				assert.Equal(t, []string{"During evaluation of flag \"test-flag\", an error was encountered in \"AfterEvaluation\" of the \"b\" hook: something bad"},
					mockLog.GetOutput(ldlog.Error))
			})
		})
	})
}

func createOrderTrackingHook(name string, tracker *orderTracker) sharedtest.TestHook {
	h := sharedtest.NewTestHook(name)
	h.BeforeInject = func(
		ctx context.Context,
		seriesContext ldhooks.EvaluationSeriesContext,
		data ldhooks.EvaluationSeriesData,
	) (ldhooks.EvaluationSeriesData, error) {
		tracker.orderBefore = append(tracker.orderBefore, name)
		return data, nil
	}
	h.AfterInject = func(
		ctx context.Context,
		seriesContext ldhooks.EvaluationSeriesContext,
		data ldhooks.EvaluationSeriesData,
		detail ldreason.EvaluationDetail,
	) (ldhooks.EvaluationSeriesData, error) {
		tracker.orderAfter = append(tracker.orderAfter, name)
		return data, nil
	}

	return h
}
