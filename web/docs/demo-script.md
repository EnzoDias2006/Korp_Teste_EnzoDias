# Final demonstration script

Target duration: 8 to 10 minutes. Record only after the Web and API candidate commits pass their
quality gates and the disposable demonstration database is ready.

## Before recording

- Open the public repository, this script, the application, and the two service health endpoints.
- Use a browser profile and terminal output that contain no credentials, tokens, or real `.env`
  values.
- Prepare two ordinary products and one product with balance `1` for the concurrency scenario.
- Keep the commands that stop and restore only the Stock Service ready in the terminal.
- Confirm the recording captures readable text, browser dialogs, and clear audio.

## 1. Architecture and quality evidence (about 1 minute)

1. Show the repository root and briefly identify `web/`, `api/`, the README, and the consolidated
   technical details.
2. Explain that the browser calls Stock for products and Billing for invoices; Billing alone
   coordinates finalization with Stock.
3. State that the UI uses standalone Angular 22 components, zoneless change detection, Signals for
   synchronous state, RxJS for HTTP flows, and typed reactive forms.
4. Show the green frontend lint, test, and production-build results without reading the whole log.

## 2. Product flow (about 1 minute)

1. Create a product with a valid code, description, and balance.
2. Return to the product list and refresh the page to prove persistence.
3. Attempt the same code again and show the safe conflict message.
4. Briefly demonstrate that invalid or fractional balance input is rejected before submit.

Narration point: frontend validation improves feedback, but Stock remains authoritative.

## 3. Multi-item invoice (about 1.5 minutes)

1. Open invoice creation and add a second line.
2. Select two different products and valid quantities.
3. Point out duplicate-product prevention and the disabled submit state while saving.
4. Submit, then show the backend-generated sequential number and `Aberta` status.
5. Return to the invoice list and open the same invoice detail.
6. Show the stored product code and description snapshots.

## 4. Finalization and print (about 1.5 minutes)

1. Click `Finalizar` once and point out the immediate `Finalizando...` indicator and disabled action.
2. Wait for Billing to return the confirmed invoice.
3. Show `Fechada`, `closed_at`, and the browser print dialog or print preview.
4. Cancel the operating-system print action if needed; the business finalization is already complete.
5. Return to products and show the balances decremented by the exact invoice quantities.
6. Reopen the closed invoice and show that the finalization action is absent.

Narration point: the UI never marks an invoice closed optimistically and calls `window.print()` only
after Billing confirms `CLOSED` with `closed_at`.

## 5. Last-unit concurrency (about 1.5 minutes)

1. Show a product with balance `1`.
2. Prepare two open invoices, each requesting that unit.
3. Finalize them concurrently using two tabs.
4. Show one confirmed `Fechada` result and one `Estoque insuficiente` result.
5. Show that the losing invoice remains `Aberta` and the product balance is exactly `0`.

Narration point: PostgreSQL locking and the Stock transaction enforce the rule; the browser only
renders each authoritative response.

## 6. Stock unavailable and manual recovery (about 2 minutes)

1. Create and open another invoice while both services are healthy.
2. Stop only the Stock Service and show its health endpoint failing.
3. Click `Finalizar` and show the processing indicator ending with the safe unavailable-service
   message and visible request ID.
4. Show that the invoice remains `Aberta` and that `Tentar novamente` is available.
5. Restore Stock and wait for its health endpoint to become healthy.
6. Use the same retry action once.
7. Show the confirmed `Fechada` state, print dialog, and a single stock decrement.

Narration point: Billing keeps the invoice recoverable, and durable idempotency prevents duplicate
consumption after retries or ambiguous responses.

## 7. Technical close (about 30 seconds)

Summarize the evidence instead of reopening source files:

- separate Stock and Billing ownership and databases;
- typed DTO mapping and invoice snapshots;
- Signals for UI state and RxJS for HTTP lifecycle cleanup;
- safe error codes plus request correlation;
- backend-authoritative concurrency, idempotency, and failure recovery;
- reproducible Bun, Go, Docker, test, and build commands in the public README.

End by displaying the three already validated public links: repository, video, and consolidated
technical details.

## Final recording check

- Watch the uploaded video from beginning to end once.
- Open the video link in an anonymous window and verify it plays without an access request.
- Confirm no credential, token, database URL, or personal notification is visible.
- Confirm the processing, concurrency, Stock-down, retry, and balance evidence are readable.
- Use the exact validated video URL in the final delivery email.
