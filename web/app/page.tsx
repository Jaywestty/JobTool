import { BACKEND_BASE, getHealth } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function Home() {
  const health = await getHealth();
  const healthy = health?.status === "ok";

  return (
    <main>
      <h1>jobtool</h1>
      <p className="lead">
        Upload a CV, answer a few questions, and get tech jobs ranked with an
        explainable match percentage. This is the scaffold: the page below calls
        the backend health endpoint.
      </p>

      <section className="card">
        <h2>Backend health</h2>
        <div className="row">
          <span>Endpoint</span>
          <span>
            <code>{BACKEND_BASE}/health</code>
          </span>
        </div>
        <div className="row">
          <span>Status</span>
          <span className={healthy ? "ok" : "bad"}>
            {health ? health.status : "unreachable"}
          </span>
        </div>
        {health?.checks
          ? Object.entries(health.checks).map(([name, state]) => (
              <div className="row" key={name}>
                <span>{name}</span>
                <span className={state === "ok" ? "ok" : "bad"}>{state}</span>
              </div>
            ))
          : null}
      </section>

      <footer>
        Coming next: CV upload, profile questions, digest frequency, application
        tracking.
      </footer>
    </main>
  );
}
