package ans

import (
	"context"
	"errors"
	"io/ioutil"
	"strconv"

	"github.com/dialogs/dialog-push-service/pkg/converter"
	"github.com/dialogs/dialog-push-service/pkg/converter/api2ans"
	"github.com/dialogs/dialog-push-service/pkg/converter/binary"
	"github.com/dialogs/dialog-push-service/pkg/provider/ans"
	"github.com/dialogs/dialog-push-service/pkg/worker"
	"go.uber.org/zap"
)

type Worker struct {
	*worker.Worker
	provider *ans.Client
}

func New(cfg *Config, logger *zap.Logger) (*Worker, error) {

	pem, err := ioutil.ReadFile(cfg.PemFile)
	if err != nil {
		return nil, err
	}

	provider, err := ans.NewFromPem(pem)
	if err != nil {
		return nil, err
	}

	var reqConverter converter.IRequestConverter

	switch cfg.ConverterKind {
	case converter.KindApi:
		reqConverter, err = api2ans.NewRequestConverter(cfg.APIConfig, provider.Certificate())
		if err != nil {
			return nil, err
		}

	case converter.KindBinary:
		reqConverter = binary.NewRequestConverter()

	}

	w := &Worker{
		provider: provider,
	}

	kind := worker.KindApns
	w.Worker = worker.New(
		cfg.Config,
		kind,
		logger.With(zap.String("worker", kind.String())),
		reqConverter,
		w.newNotification,
		w.sendNotification,
	)

	return w, nil
}

func (w *Worker) newNotification() interface{} {
	return &ans.Request{}
}

func (w *Worker) sendNotification(ctx context.Context, token string, out interface{}) error {

	req, ok := out.(*ans.Request)
	if !ok {
		return worker.ErrInvalidOutDataType
	}

	req.Token = token

	answer, err := w.provider.Send(ctx, req)
	if err != nil {
		return err

	} else if answer.StatusCode != 200 {
		return errors.New(strconv.Itoa(answer.StatusCode) + " " + answer.Reason)
	}

	return nil
}