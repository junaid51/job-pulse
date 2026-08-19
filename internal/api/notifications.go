package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// notification is one match event: a job, the profile it matched, and whether it
// has been looked at.
type notification struct {
	ProfileID   int64  `json:"profile_id"`
	ProfileName string `json:"profile_name"`
	Job         job    `json:"job"`
}

// listNotifications is the match feed across every profile, newest first. The
// Jobs screen asks "what matches this profile"; this asks "what happened".
func listNotifications(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := intParam(r, "limit", defaultLimit, maxLimit)

		var at any
		var atID any
		if raw := r.URL.Query().Get("cursor"); raw != "" {
			t, id, err := parseCursor(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			at, atID = t, id
		}

		rows, err := pool.Query(r.Context(), `
			select m.profile_id, p.name,
			       j.id, j.provider, j.company, j.title, j.location, j.remote, j.url,
			       j.posted_at, m.created_at, m.seen_at
			from matches m
			join jobs j on j.id = m.job_id
			join profiles p on p.id = m.profile_id and p.owner = $4
			where $1::timestamptz is null or (m.created_at, j.id) < ($1, $2::bigint)
			order by m.created_at desc, j.id desc
			limit $3`, at, atID, limit, deviceID(r))
		if err != nil {
			serverError(w, "listing notifications", err)
			return
		}
		defer rows.Close()

		notifications := []notification{}
		for rows.Next() {
			var n notification
			if err := rows.Scan(&n.ProfileID, &n.ProfileName,
				&n.Job.ID, &n.Job.Provider, &n.Job.Company, &n.Job.Title, &n.Job.Location,
				&n.Job.Remote, &n.Job.URL, &n.Job.PostedAt, &n.Job.MatchedAt, &n.Job.SeenAt); err != nil {
				serverError(w, "reading notifications", err)
				return
			}
			notifications = append(notifications, n)
		}
		if err := rows.Err(); err != nil {
			serverError(w, "reading notifications", err)
			return
		}

		var unread int
		if err := pool.QueryRow(r.Context(), `
			select count(*) from matches m
			join profiles p on p.id = m.profile_id and p.owner = $1
			where m.seen_at is null`, deviceID(r)).Scan(&unread); err != nil {
			serverError(w, "counting unread", err)
			return
		}

		body := map[string]any{"notifications": notifications, "unread": unread}
		if len(notifications) == limit {
			body["next_cursor"] = formatCursor(notifications[len(notifications)-1].Job)
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// markSeen marks everything as read. There is one user with one phone, so
// "opened the Notifications screen" means all of it — no ids to plumb through.
func markSeen(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag, err := pool.Exec(r.Context(), `
			update matches m set seen_at = $1
			from profiles p
			where p.id = m.profile_id and p.owner = $2 and m.seen_at is null`,
			time.Now(), deviceID(r))
		if err != nil {
			serverError(w, "marking seen", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"marked": tag.RowsAffected()})
	}
}
