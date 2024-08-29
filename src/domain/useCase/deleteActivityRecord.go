package useCase

import (
	"errors"
	"log/slog"

	"github.com/speedianet/os/src/domain/dto"
	"github.com/speedianet/os/src/domain/repository"
)

func DeleteActivityRecord(
	activityRecordCmdRepo repository.ActivityRecordCmdRepo,
	deleteDto dto.DeleteActivityRecords,
) error {
	err := activityRecordCmdRepo.Delete(deleteDto)
	if err != nil {
		slog.Error("DeleteActivityRecordError", slog.Any("err", err))
		return errors.New("DeleteActivityRecordInfraError")
	}

	return nil
}