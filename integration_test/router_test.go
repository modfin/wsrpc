package integration_test

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/modfin/wsrpc"
)

// Example
func setupRouter() *wsrpc.Router {
	router := wsrpc.NewRouter()

	router.SetHandler("add", func(ctx wsrpc.Context) (err error) {
		a := ctx.Request().Header.Get("A").IntOr(0)
		b := ctx.Request().Header.Get("B").IntOr(0)

		select {
		case <-ctx.Done():
			return wsrpc.ErrContextCancelled
		default:
		}

		ctx.Response().Result, err = json.Marshal(a + b)
		if err != nil {
			return err
		}

		return nil
	})

	router.SetHandler("square", func(ctx wsrpc.Context) (err error) {
		var params struct {
			Val int `json:"val"`
		}
		err = json.Unmarshal(ctx.Request().Params, &params)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return wsrpc.ErrContextCancelled
		default:
		}

		ctx.Response().Result, err = json.Marshal(math.Pow(float64(params.Val), 2))
		if err != nil {
			return err
		}

		return nil
	})

	router.SetStream("countdown", func(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) (err error) {
		s := ctx.Request().Header.Get("state").IntOr(0)

		for s > 0 {
			select {
			case <-ctx.Done():
				return errors.New("context cancelled")
			default:
			}

			rsp := ctx.NewResponse()
			rsp.Result, err = json.Marshal(s)
			if err != nil {
				ctx.Response().Error = wsrpc.ServerError(errors.New("failed to encode response"))
				return err
			}

			s--
			rsp.Header.Set("state", s)

			err = ch.Write(rsp)
			if err != nil {
				ctx.Response().Error = wsrpc.ServerError(errors.New("failed to write response"))
				return err
			}
		}

		return nil
	})

	router.SetStream("reminder", func(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) (err error) {
		if ctx.Request().Header.Get("state").BoolOr(false) {
			return nil
		}

		var reminder struct {
			When time.Time `json:"time"`
			Msg  string    `json:"msg"`
		}
		err = json.Unmarshal(ctx.Request().Params, &reminder)
		if err != nil {
			return err
		}

		waitFor := time.Until(reminder.When)
		if waitFor < 0 {
			waitFor = 0
		}

		timer := time.After(waitFor)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer:
			rsp := ctx.NewResponse()
			rsp.Result, err = json.Marshal(reminder.Msg)
			if err != nil {
				return err
			}

			rsp.Header.Set("state", true)

			err = ch.Write(rsp)
			if err != nil {
				ctx.Response().Error = wsrpc.ServerError(errors.New("failed to write response"))

				return err
			}
		}

		return nil
	})

	return router
}
