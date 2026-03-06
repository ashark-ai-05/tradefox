package handlers

import (
	"net/http"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// GetStudies returns all current study values.
//
//	GET /api/studies
func GetStudies(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var studies []models.BaseStudyModel

		deps.DataStore.studies.Range(func(_, value any) bool {
			studies = append(studies, value.(models.BaseStudyModel))
			return true
		})

		if studies == nil {
			studies = []models.BaseStudyModel{}
		}

		writeJSON(w, http.StatusOK, studies)
	}
}
