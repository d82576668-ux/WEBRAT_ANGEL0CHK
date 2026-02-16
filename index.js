try {
  process.env.NODE_ENV = process.env.NODE_ENV || "production";
  require("./dist/src/index.js");
} catch (e) {
  console.error("Failed to start: dist build not found. Ensure 'npm run build' ran successfully.");
  console.error(e && e.message ? e.message : e);
  process.exit(1);
}
