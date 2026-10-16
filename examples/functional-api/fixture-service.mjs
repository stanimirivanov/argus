import http from "node:http";

export function startFixtureService({ missedFailure = false } = {}) {
  const server = http.createServer(async (request, response) => {
    const body = (status, value) => {
      response.writeHead(status, { "content-type": "application/json" });
      response.end(JSON.stringify(value));
    };
    if (request.method === "POST" && request.url === "/orders") {
      body(201, { id: "order-1" });
    } else if (request.method === "GET" && request.url === "/orders") {
      body(200, { items: missedFailure ? [] : [{ id: "order-1" }] });
    } else if (request.method === "GET" && request.url === "/billing/health") {
      body(200, { healthy: true });
    } else {
      body(404, { error: "not-found" });
    }
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      resolve({
        url: `http://127.0.0.1:${address.port}`,
        close: () =>
          new Promise((done, fail) => server.close((error) => (error ? fail(error) : done()))),
      });
    });
  });
}
