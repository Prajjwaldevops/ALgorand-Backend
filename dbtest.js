const { Client } = require('pg');
require('dotenv').config({ path: '../.env' });

const run = async () => {
  const client = new Client({
    connectionString: process.env.DATABASE_URL,
  });
  await client.connect();

  try {
    const { rows } = await client.query('SELECT id, dispute_id, bounty_id FROM disputes ORDER BY created_at DESC LIMIT 5');
    console.log("Recent disputes:", rows);
    if (rows.length > 0) {
      const targetId = rows[0].id;
      console.log(`\nTesting GetDisputeDetail query for ${targetId}:`);
      
      const q = `
        SELECT d.id, CAST(d.id AS TEXT) as id_text, d.dispute_id, d.bounty_id,
               d.freelancer_id, d.initiated_by, d.creator_id,
               b.title
        FROM disputes d
        JOIN bounties b ON d.bounty_id = b.id
        LEFT JOIN profiles pf ON COALESCE(d.freelancer_id, d.initiated_by) = pf.id
        LEFT JOIN profiles pc ON d.creator_id = pc.id
        WHERE CAST(d.id AS TEXT) = $1 OR d.dispute_id = $1
      `;
      const res = await client.query(q, [targetId]);
      console.log(`Results: ${res.rowCount} row(s)`, res.rows);
    }
  } catch(e) {
    console.error("Error:", e);
  } finally {
    await client.end();
  }
};
run();
