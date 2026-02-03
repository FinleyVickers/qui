// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package arr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
)

func TestClient_Ping(t *testing.T) {
	tests := []struct {
		name           string
		responseCode   int
		responseBody   string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:         "successful ping",
			responseCode: http.StatusOK,
			responseBody: `{"appName":"Sonarr","version":"4.0.0.123"}`,
			wantErr:      false,
		},
		{
			name:           "unauthorized",
			responseCode:   http.StatusUnauthorized,
			responseBody:   `{"error":"Unauthorized"}`,
			wantErr:        true,
			wantErrContain: "authentication failed",
		},
		{
			name:           "server error",
			responseCode:   http.StatusInternalServerError,
			responseBody:   `Internal Server Error`,
			wantErr:        true,
			wantErrContain: "unexpected status 500",
		},
		{
			name:           "empty appName",
			responseCode:   http.StatusOK,
			responseBody:   `{"appName":"","version":"4.0.0"}`,
			wantErr:        true,
			wantErrContain: "missing appName",
		},
		{
			name:           "invalid JSON",
			responseCode:   http.StatusOK,
			responseBody:   `not json`,
			wantErr:        true,
			wantErrContain: "failed to decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v3/system/status", r.URL.Path)
				assert.Equal(t, "test-api-key", r.Header.Get("X-Api-Key"))
				w.WriteHeader(tt.responseCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key", models.ArrInstanceTypeSonarr, 15)
			err := client.Ping(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContain)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_ParseTitle_Sonarr(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		responseBody string
		wantIDs      *models.ExternalIDs
		wantErr      bool
	}{
		{
			name:         "full IDs from series",
			responseCode: http.StatusOK,
			responseBody: `{
				"title": "Breaking Bad S01E01",
				"parsedEpisodeInfo": {"seriesTitle": "Breaking Bad"},
				"series": {
					"id": 123,
					"title": "Breaking Bad",
					"tvdbId": 81189,
					"tvMazeId": 169,
					"tmdbId": 1396,
					"imdbId": "tt0903747"
				}
			}`,
			wantIDs: &models.ExternalIDs{
				TVDbID:   81189,
				TVMazeID: 169,
				TMDbID:   1396,
				IMDbID:   "tt0903747",
			},
			wantErr: false,
		},
		{
			name:         "partial IDs - only TVDb",
			responseCode: http.StatusOK,
			responseBody: `{
				"title": "Some Show S01E01",
				"series": {"tvdbId": 12345}
			}`,
			wantIDs: &models.ExternalIDs{
				TVDbID: 12345,
			},
			wantErr: false,
		},
		{
			name:         "nil series returns nil IDs",
			responseCode: http.StatusOK,
			responseBody: `{
				"title": "Unknown Show S01E01",
				"parsedEpisodeInfo": {"seriesTitle": "Unknown Show"},
				"series": null
			}`,
			wantIDs: nil,
			wantErr: false,
		},
		{
			name:         "series with zero IDs returns nil",
			responseCode: http.StatusOK,
			responseBody: `{
				"title": "Empty Show",
				"series": {"id": 1, "tvdbId": 0, "tvMazeId": 0, "tmdbId": 0, "imdbId": ""}
			}`,
			wantIDs: nil,
			wantErr: false,
		},
		{
			name:         "series with imdbId as 0 string ignored",
			responseCode: http.StatusOK,
			responseBody: `{
				"title": "Show",
				"series": {"tvdbId": 999, "imdbId": "0"}
			}`,
			wantIDs: &models.ExternalIDs{
				TVDbID: 999,
			},
			wantErr: false,
		},
		{
			name:         "unauthorized",
			responseCode: http.StatusUnauthorized,
			responseBody: ``,
			wantIDs:      nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v3/parse", r.URL.Path)
				assert.NotEmpty(t, r.URL.Query().Get("title"))
				w.WriteHeader(tt.responseCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key", models.ArrInstanceTypeSonarr, 15)
			ids, err := client.ParseTitle(context.Background(), "Test Title")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestClient_ParseTitle_Radarr(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		wantIDs      *models.ExternalIDs
	}{
		{
			name: "full IDs from movie",
			responseBody: `{
				"title": "Inception (2010)",
				"parsedMovieInfo": {"movieTitle": "Inception", "year": 2010},
				"movie": {
					"id": 456,
					"title": "Inception",
					"tmdbId": 27205,
					"imdbId": "tt1375666"
				}
			}`,
			wantIDs: &models.ExternalIDs{
				TMDbID: 27205,
				IMDbID: "tt1375666",
			},
		},
		{
			name: "IDs from parsedMovieInfo when movie is nil",
			responseBody: `{
				"title": "Movie.2020.tt1234567.1080p",
				"parsedMovieInfo": {
					"movieTitle": "Movie",
					"year": 2020,
					"imdbId": "tt1234567",
					"tmdbId": 99999
				},
				"movie": null
			}`,
			wantIDs: &models.ExternalIDs{
				TMDbID: 99999,
				IMDbID: "tt1234567",
			},
		},
		{
			name: "movie IDs take precedence over parsedMovieInfo",
			responseBody: `{
				"title": "Film",
				"parsedMovieInfo": {"imdbId": "tt0000001", "tmdbId": 1},
				"movie": {"tmdbId": 2, "imdbId": "tt0000002"}
			}`,
			wantIDs: &models.ExternalIDs{
				TMDbID: 2,
				IMDbID: "tt0000002",
			},
		},
		{
			name: "fallback to parsedMovieInfo for missing movie fields",
			responseBody: `{
				"title": "Film",
				"parsedMovieInfo": {"imdbId": "tt1111111", "tmdbId": 111},
				"movie": {"tmdbId": 222, "imdbId": ""}
			}`,
			wantIDs: &models.ExternalIDs{
				TMDbID: 222,
				IMDbID: "tt1111111",
			},
		},
		{
			name: "nil movie and empty parsedMovieInfo returns nil",
			responseBody: `{
				"title": "Unknown",
				"parsedMovieInfo": {"movieTitle": "Unknown"},
				"movie": null
			}`,
			wantIDs: nil,
		},
		{
			name: "zero values ignored in parsedMovieInfo",
			responseBody: `{
				"title": "Zero",
				"parsedMovieInfo": {"imdbId": "0", "tmdbId": 0},
				"movie": null
			}`,
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key", models.ArrInstanceTypeRadarr, 15)
			ids, err := client.ParseTitle(context.Background(), "Test")

			require.NoError(t, err)
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestSonarrParseResponse_ExtractExternalIDs(t *testing.T) {
	tests := []struct {
		name     string
		response SonarrParseResponse
		want     *models.ExternalIDs
	}{
		{
			name: "all IDs present",
			response: SonarrParseResponse{
				Series: &SonarrSeries{
					TVDbID:   81189,
					TVMazeID: 169,
					TMDbID:   1396,
					IMDbID:   "tt0903747",
				},
			},
			want: &models.ExternalIDs{
				TVDbID:   81189,
				TVMazeID: 169,
				TMDbID:   1396,
				IMDbID:   "tt0903747",
			},
		},
		{
			name: "nil series",
			response: SonarrParseResponse{
				Series: nil,
			},
			want: nil,
		},
		{
			name: "all zero values",
			response: SonarrParseResponse{
				Series: &SonarrSeries{
					TVDbID:   0,
					TVMazeID: 0,
					TMDbID:   0,
					IMDbID:   "",
				},
			},
			want: nil,
		},
		{
			name: "imdb as '0' string ignored",
			response: SonarrParseResponse{
				Series: &SonarrSeries{
					TVDbID: 123,
					IMDbID: "0",
				},
			},
			want: &models.ExternalIDs{
				TVDbID: 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.ExtractExternalIDs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRadarrParseResponse_ExtractExternalIDs(t *testing.T) {
	tests := []struct {
		name     string
		response RadarrParseResponse
		want     *models.ExternalIDs
	}{
		{
			name: "IDs from movie",
			response: RadarrParseResponse{
				Movie: &RadarrMovie{
					TMDbID: 27205,
					IMDbID: "tt1375666",
				},
			},
			want: &models.ExternalIDs{
				TMDbID: 27205,
				IMDbID: "tt1375666",
			},
		},
		{
			name: "IDs from parsedMovieInfo when movie is nil",
			response: RadarrParseResponse{
				ParsedMovieInfo: &RadarrParsedMovieInfo{
					TMDbID: 12345,
					IMDbID: "tt9999999",
				},
				Movie: nil,
			},
			want: &models.ExternalIDs{
				TMDbID: 12345,
				IMDbID: "tt9999999",
			},
		},
		{
			name: "movie takes precedence",
			response: RadarrParseResponse{
				ParsedMovieInfo: &RadarrParsedMovieInfo{
					TMDbID: 1,
					IMDbID: "tt0000001",
				},
				Movie: &RadarrMovie{
					TMDbID: 2,
					IMDbID: "tt0000002",
				},
			},
			want: &models.ExternalIDs{
				TMDbID: 2,
				IMDbID: "tt0000002",
			},
		},
		{
			name: "fallback for missing movie fields only",
			response: RadarrParseResponse{
				ParsedMovieInfo: &RadarrParsedMovieInfo{
					TMDbID: 111,
					IMDbID: "tt1111111",
				},
				Movie: &RadarrMovie{
					TMDbID: 222,
					IMDbID: "", // empty, should fallback
				},
			},
			want: &models.ExternalIDs{
				TMDbID: 222,
				IMDbID: "tt1111111",
			},
		},
		{
			name: "parsedMovieInfo imdb '0' ignored",
			response: RadarrParseResponse{
				ParsedMovieInfo: &RadarrParsedMovieInfo{
					TMDbID: 555,
					IMDbID: "0",
				},
			},
			want: &models.ExternalIDs{
				TMDbID: 555,
			},
		},
		{
			name:     "both nil returns nil",
			response: RadarrParseResponse{},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.ExtractExternalIDs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8989/", "apikey123", models.ArrInstanceTypeSonarr, 30)

	assert.Equal(t, "http://localhost:8989", client.BaseURL()) // trailing slash trimmed
	assert.Equal(t, models.ArrInstanceTypeSonarr, client.InstanceType())
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	client := NewClient("http://localhost:8989", "key", models.ArrInstanceTypeRadarr, 0)

	// Default timeout should be 15 seconds
	assert.Equal(t, defaultTimeout, client.timeout)
}

func TestClient_GetMovieFiles(t *testing.T) {
	tests := []struct {
		name         string
		instanceType models.ArrInstanceType
		responseCode int
		responseBody string
		wantFiles    []RadarrMovieFile
		wantErr      bool
		errContains  string
	}{
		{
			name:         "successful fetch with scene names",
			instanceType: models.ArrInstanceTypeRadarr,
			responseCode: http.StatusOK,
			responseBody: `[
				{
					"id": 1,
					"movieId": 100,
					"relativePath": "Movie.Title.2024.1080p.BluRay.x264-GROUP.mkv",
					"path": "/movies/Movie Title (2024)/Movie.Title.2024.1080p.BluRay.x264-GROUP.mkv",
					"size": 8589934592,
					"sceneName": "Movie.Title.2024.1080p.BluRay.x264-GROUP"
				},
				{
					"id": 2,
					"movieId": 101,
					"relativePath": "Another.Movie.2023.2160p.WEB-DL.x265-TEAM.mkv",
					"path": "/movies/Another Movie (2023)/Another.Movie.2023.2160p.WEB-DL.x265-TEAM.mkv",
					"size": 17179869184,
					"sceneName": "Another.Movie.2023.2160p.WEB-DL.x265-TEAM"
				}
			]`,
			wantFiles: []RadarrMovieFile{
				{
					ID:           1,
					MovieID:      100,
					RelativePath: "Movie.Title.2024.1080p.BluRay.x264-GROUP.mkv",
					Path:         "/movies/Movie Title (2024)/Movie.Title.2024.1080p.BluRay.x264-GROUP.mkv",
					Size:         8589934592,
					SceneName:    "Movie.Title.2024.1080p.BluRay.x264-GROUP",
				},
				{
					ID:           2,
					MovieID:      101,
					RelativePath: "Another.Movie.2023.2160p.WEB-DL.x265-TEAM.mkv",
					Path:         "/movies/Another Movie (2023)/Another.Movie.2023.2160p.WEB-DL.x265-TEAM.mkv",
					Size:         17179869184,
					SceneName:    "Another.Movie.2023.2160p.WEB-DL.x265-TEAM",
				},
			},
			wantErr: false,
		},
		{
			name:         "empty result",
			instanceType: models.ArrInstanceTypeRadarr,
			responseCode: http.StatusOK,
			responseBody: `[]`,
			wantFiles:    []RadarrMovieFile{},
			wantErr:      false,
		},
		{
			name:         "unauthorized",
			instanceType: models.ArrInstanceTypeRadarr,
			responseCode: http.StatusUnauthorized,
			responseBody: `{"error":"Unauthorized"}`,
			wantErr:      true,
			errContains:  "authentication failed",
		},
		{
			name:         "wrong instance type",
			instanceType: models.ArrInstanceTypeSonarr,
			wantErr:      true,
			errContains:  "only supported for Radarr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.instanceType == models.ArrInstanceTypeRadarr && tt.responseCode > 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v3/moviefile", r.URL.Path)
					assert.Equal(t, "test-api-key", r.Header.Get("X-Api-Key"))
					w.WriteHeader(tt.responseCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = NewClient(server.URL, "test-api-key", tt.instanceType, 15)
			} else {
				client = NewClient("http://localhost:7878", "test-api-key", tt.instanceType, 15)
			}

			files, err := client.GetMovieFiles(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFiles, files)
			}
		})
	}
}

func TestClient_GetEpisodeFiles(t *testing.T) {
	tests := []struct {
		name         string
		instanceType models.ArrInstanceType
		responseCode int
		responseBody string
		wantFiles    []SonarrEpisodeFile
		wantErr      bool
		errContains  string
	}{
		{
			name:         "successful fetch with scene names",
			instanceType: models.ArrInstanceTypeSonarr,
			responseCode: http.StatusOK,
			responseBody: `[
				{
					"id": 1,
					"seriesId": 200,
					"seasonNumber": 1,
					"relativePath": "Season 01/Show.Name.S01E01.720p.HDTV.x264-GROUP.mkv",
					"path": "/tv/Show Name/Season 01/Show.Name.S01E01.720p.HDTV.x264-GROUP.mkv",
					"size": 1073741824,
					"sceneName": "Show.Name.S01E01.720p.HDTV.x264-GROUP"
				},
				{
					"id": 2,
					"seriesId": 200,
					"seasonNumber": 1,
					"relativePath": "Season 01/Show.Name.S01E02.720p.HDTV.x264-GROUP.mkv",
					"path": "/tv/Show Name/Season 01/Show.Name.S01E02.720p.HDTV.x264-GROUP.mkv",
					"size": 1073741824,
					"sceneName": "Show.Name.S01E02.720p.HDTV.x264-GROUP"
				}
			]`,
			wantFiles: []SonarrEpisodeFile{
				{
					ID:           1,
					SeriesID:     200,
					SeasonNumber: 1,
					RelativePath: "Season 01/Show.Name.S01E01.720p.HDTV.x264-GROUP.mkv",
					Path:         "/tv/Show Name/Season 01/Show.Name.S01E01.720p.HDTV.x264-GROUP.mkv",
					Size:         1073741824,
					SceneName:    "Show.Name.S01E01.720p.HDTV.x264-GROUP",
				},
				{
					ID:           2,
					SeriesID:     200,
					SeasonNumber: 1,
					RelativePath: "Season 01/Show.Name.S01E02.720p.HDTV.x264-GROUP.mkv",
					Path:         "/tv/Show Name/Season 01/Show.Name.S01E02.720p.HDTV.x264-GROUP.mkv",
					Size:         1073741824,
					SceneName:    "Show.Name.S01E02.720p.HDTV.x264-GROUP",
				},
			},
			wantErr: false,
		},
		{
			name:         "empty result",
			instanceType: models.ArrInstanceTypeSonarr,
			responseCode: http.StatusOK,
			responseBody: `[]`,
			wantFiles:    []SonarrEpisodeFile{},
			wantErr:      false,
		},
		{
			name:         "unauthorized",
			instanceType: models.ArrInstanceTypeSonarr,
			responseCode: http.StatusUnauthorized,
			responseBody: `{"error":"Unauthorized"}`,
			wantErr:      true,
			errContains:  "authentication failed",
		},
		{
			name:         "wrong instance type",
			instanceType: models.ArrInstanceTypeRadarr,
			wantErr:      true,
			errContains:  "only supported for Sonarr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.instanceType == models.ArrInstanceTypeSonarr && tt.responseCode > 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v3/episodefile", r.URL.Path)
					assert.Equal(t, "test-api-key", r.Header.Get("X-Api-Key"))
					w.WriteHeader(tt.responseCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = NewClient(server.URL, "test-api-key", tt.instanceType, 15)
			} else {
				client = NewClient("http://localhost:8989", "test-api-key", tt.instanceType, 15)
			}

			files, err := client.GetEpisodeFiles(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFiles, files)
			}
		})
	}
}
