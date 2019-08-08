package starts

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nyaruka/goflow/utils/uuids"
	_ "github.com/nyaruka/mailroom/hooks"
	"github.com/nyaruka/mailroom/models"
	"github.com/nyaruka/mailroom/queue"
	"github.com/nyaruka/mailroom/runner"
	"github.com/nyaruka/mailroom/search"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
)

func TestStarts(t *testing.T) {
	testsuite.Reset()
	ctx := testsuite.CTX()
	rp := testsuite.RP()
	db := testsuite.DB()
	rc := testsuite.RC()
	defer rc.Close()

	mes := search.NewMockElasticServer()
	defer mes.Close()

	es, err := elastic.NewClient(
		elastic.SetURL(mes.URL()),
		elastic.SetHealthcheck(false),
		elastic.SetSniff(false),
	)
	assert.NoError(t, err)

	// insert a flow run for one of our contacts
	// TODO: can be replaced with a normal flow start of another flow once we support flows with waits
	db.MustExec(
		`INSERT INTO flows_flowrun(uuid, is_active, created_on, modified_on, responded, contact_id, flow_id, org_id)
		                    VALUES($1, TRUE, now(), now(), FALSE, $2, $3, 1);`, uuids.New(), models.GeorgeID, models.SingleMessageFlowID)

	tcs := []struct {
		FlowID              models.FlowID
		GroupIDs            []models.GroupID
		ContactIDs          []models.ContactID
		Query               string
		QueryResponse       string
		RestartParticipants models.RestartParticipants
		IncludeActive       models.IncludeActive
		Queue               string
		ContactCount        int
		BatchCount          int
		TotalCount          int
	}{
		{models.SingleMessageFlowID, nil, nil, "", "", false, false, queue.BatchQueue, 0, 0, 0},
		{models.SingleMessageFlowID, []models.GroupID{models.DoctorsGroupID}, nil, "", "", false, false, queue.BatchQueue, 121, 2, 121},
		{models.SingleMessageFlowID, []models.GroupID{models.DoctorsGroupID}, []models.ContactID{models.CathyID}, "", "", false, false, queue.BatchQueue, 121, 2, 0},
		{models.SingleMessageFlowID, nil, []models.ContactID{models.CathyID}, "", "", true, true, queue.HandlerQueue, 1, 1, 1},
		{models.SingleMessageFlowID, []models.GroupID{models.DoctorsGroupID}, []models.ContactID{models.BobID}, "", "", false, false, queue.BatchQueue, 122, 2, 1},
		{models.SingleMessageFlowID, nil, []models.ContactID{models.BobID}, "", "", false, false, queue.HandlerQueue, 1, 1, 0},
		{models.SingleMessageFlowID, nil, []models.ContactID{models.BobID}, "", "", false, true, queue.HandlerQueue, 1, 1, 0},
		{models.SingleMessageFlowID, nil, []models.ContactID{models.BobID}, "", "", true, true, queue.HandlerQueue, 1, 1, 1},
		{models.SingleMessageFlowID, nil, nil, "bob", fmt.Sprintf(`{
			"_scroll_id": "DXF1ZXJ5QW5kRmV0Y2gBAAAAAAAbgc0WS1hqbHlfb01SM2lLTWJRMnVOSVZDdw==",
			"took": 2,
			"timed_out": false,
			"_shards": {
			  "total": 1,
			  "successful": 1,
			  "skipped": 0,
			  "failed": 0
			},
			"hits": {
			  "total": 1,
			  "max_score": null,
			  "hits": [
				{
				  "_index": "contacts",
				  "_type": "_doc",
				  "_id": "%d",
				  "_score": null,
				  "_routing": "1",
				  "sort": [
					15124352
				  ]
				}
			  ]
			}
		}`, models.BobID), true, true, queue.HandlerQueue, 1, 1, 1},
	}

	for i, tc := range tcs {
		mes.NextResponse = tc.QueryResponse

		// handle our start task
		start := models.NewFlowStart(models.Org1, models.MessagingFlow, tc.FlowID, tc.RestartParticipants, tc.IncludeActive).
			WithGroupIDs(tc.GroupIDs).
			WithContactIDs(tc.ContactIDs).
			WithQuery(tc.Query)

		err := models.InsertFlowStarts(ctx, db, []*models.FlowStart{start})
		assert.NoError(t, err)

		err = CreateFlowBatches(ctx, db, rp, es, start)
		assert.NoError(t, err)

		// pop all our tasks and execute them
		var task *queue.Task
		count := 0
		for {
			task, err = queue.PopNextTask(rc, tc.Queue)
			assert.NoError(t, err)
			if task == nil {
				break
			}

			count++
			assert.Equal(t, queue.StartFlowBatch, task.Type)
			batch := &models.FlowStartBatch{}
			err = json.Unmarshal(task.Task, batch)
			assert.NoError(t, err)

			_, err = runner.StartFlowBatch(ctx, db, rp, batch)
			assert.NoError(t, err)
		}

		// assert our count of batches
		assert.Equal(t, tc.BatchCount, count, "%d: unexpected batch count", i)

		// assert our count of total flow runs created
		testsuite.AssertQueryCount(t, db, `SELECT count(*) FROM flows_flowrun where flow_id = $1 AND start_id = $2 AND is_active = FALSE`,
			[]interface{}{tc.FlowID, start.ID()}, tc.TotalCount, "%d: unexpected total run count", i)

		// flow start should be complete
		testsuite.AssertQueryCount(t, db, `SELECT count(*) FROM flows_flowstart where status = 'C' AND id = $1 AND contact_count = $2`,
			[]interface{}{start.ID(), tc.ContactCount}, 1, "%d: start status not set to complete", i)
	}
}
