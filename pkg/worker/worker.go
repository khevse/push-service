package worker

import (
	"context"
	"errors"

	"github.com/dialogs/dialog-push-service/pkg/converter"
	"go.uber.org/zap"
)

var (
	ErrInvalidDeviceToken   = errors.New("empty device token")
	ErrUnknownResponseError = errors.New("unknown response error")
	ErrInvalidOutDataType   = errors.New("invalid out data type")
)

type FnNewNotification func() interface{}
type FnSendNotification func(ctx context.Context, token string, out interface{}) error

type Worker struct {
	projectID          string
	kind               Kind
	nopMode            bool
	threads            chan struct{}
	logger             *zap.Logger
	reqConverter       converter.IRequestConverter
	fnNewNotification  FnNewNotification
	fnSendNotification FnSendNotification
}

func New(
	cfg *Config,
	kind Kind,
	logger *zap.Logger,
	reqConverter converter.IRequestConverter,
	fnNewNotification FnNewNotification,
	fnSendNotification FnSendNotification,
) *Worker {

	countThreads := cfg.CountThreads
	if countThreads <= 0 {
		countThreads = 1
	}

	threads := make(chan struct{}, countThreads)
	for i := 0; i < countThreads; i++ {
		threads <- struct{}{}
	}

	return &Worker{
		projectID:          cfg.ProjectID,
		kind:               kind,
		nopMode:            cfg.NopMode,
		threads:            threads,
		logger:             logger,
		reqConverter:       reqConverter,
		fnNewNotification:  fnNewNotification,
		fnSendNotification: fnSendNotification,
	}
}

func (w *Worker) Kind() Kind {
	return w.kind
}

func (w *Worker) ProviderID() string {
	return w.projectID
}

func (w *Worker) NoOpMode() bool {
	return w.nopMode
}

func (w *Worker) Send(ctx context.Context, req *Request) <-chan *Response {

	ch := make(chan *Response)
	reserved := <-w.threads

	go func() {
		defer func() { w.threads <- reserved }()
		defer close(ch)

		if len(req.Devices) == 0 {
			w.logger.Error(ErrInvalidDeviceToken.Error())

			ch <- &Response{
				ProjectID: w.projectID,
				Error:     ErrInvalidDeviceToken,
			}
			return
		}

		out := w.fnNewNotification()
		err := w.reqConverter.Convert(req.Payload, out)

		for _, token := range req.Devices {
			resp := &Response{
				ProjectID:   w.projectID,
				DeviceToken: token,
			}

			// hide device token to hash
			l := w.logger.With(zap.String("token hash", TokenHash(token)))

			if err != nil {
				// convert error
				l.Error("convert incoming message", zap.Error(err))
				resp.Error = err

			} else if w.nopMode {
				l.Info("nop mode", zap.Any("send notification", resp))

			} else {

				err := w.fnSendNotification(ctx, token, out)
				if err != nil {
					resp.Error = err
					l.Error("failed to send", zap.Error(resp.Error))
				} else {
					l.Info("success send")
				}
			}

			ch <- resp
		}
	}()

	return ch
}