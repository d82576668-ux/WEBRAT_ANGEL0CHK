"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = require("express");
const db_1 = require("../db");
const router = (0, express_1.Router)();
router.get("/:id", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, device_id, type, status, payload, result, created_at, updated_at from tasks where id = $1", [id]);
    if (!rows[0]) {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json(rows[0]);
});
router.patch("/:id", async (req, res) => {
    const id = String(req.params.id);
    const { status, result } = req.body || {};
    const allowed = new Set(["queued", "running", "done", "error"]);
    if (!status || !allowed.has(String(status))) {
        res.status(400).json({ error: "invalid status" });
        return;
    }
    const rows = await (0, db_1.query)("update tasks set status = $1, result = $2, updated_at = now() where id = $3 returning id, device_id, type, status, result, updated_at", [String(status), result ?? null, id]);
    if (!rows[0]) {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json(rows[0]);
});
exports.default = router;
