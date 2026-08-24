import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  inject,
  OnInit,
  signal,
  computed,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpErrorResponse } from '@angular/common/http';
import {
  BehaviorSubject,
  combineLatest,
  finalize,
  switchMap,
  catchError,
  of,
  filter,
  tap,
  map,
} from 'rxjs';

import { MatCardModule } from '@angular/material/card';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatIconModule } from '@angular/material/icon';
import { MatButtonModule } from '@angular/material/button';
import { MatTableModule } from '@angular/material/table';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { MatTooltipModule } from '@angular/material/tooltip';

import { InvoiceApiService } from '../../services/invoice-api.service';
import { Invoice } from '../../../../core/models/invoice.model';
import { extractSafeHttpApiError, SafeApiError } from '../../../../core/models/api-error.model';

@Component({
  selector: 'app-invoice-detail',
  standalone: true,
  imports: [
    CommonModule,
    RouterModule,
    MatCardModule,
    MatProgressSpinnerModule,
    MatIconModule,
    MatButtonModule,
    MatTableModule,
    MatSnackBarModule,
    MatTooltipModule,
  ],
  templateUrl: './invoice-detail.html',
  styleUrl: './invoice-detail.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class InvoiceDetail implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly invoiceApi = inject(InvoiceApiService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly snackBar = inject(MatSnackBar);

  readonly loading = signal(true);
  readonly invoice = signal<Invoice | null>(null);
  readonly notFound = signal(false);
  readonly error = signal<string | null>(null);
  readonly printing = signal(false);
  readonly actionError = signal<SafeApiError | null>(null);
  readonly requestId = signal<string | null>(null);

  readonly displayedColumns: string[] = ['productCode', 'productDescription', 'quantity'];

  readonly canPrint = computed(() => {
    const inv = this.invoice();
    return inv !== null && inv.status === 'OPEN' && !this.printing();
  });

  private readonly retrySubject = new BehaviorSubject<void>(undefined);

  ngOnInit(): void {
    combineLatest([
      this.route.paramMap.pipe(map((params) => params.get('id'))),
      this.retrySubject.asObservable(),
    ])
      .pipe(
        filter(([id]) => id !== null),
        tap(() => {
          this.loading.set(true);
          this.error.set(null);
          this.notFound.set(false);
          this.actionError.set(null);
          this.invoice.set(null);
        }),
        switchMap(([id]) => {
          return this.invoiceApi.get(id as string).pipe(
            finalize(() => this.loading.set(false)),
            catchError((err: HttpErrorResponse) => {
              const safeError = extractSafeHttpApiError(err);
              if (safeError.code === 'INVOICE_NOT_FOUND' || err.status === 404) {
                this.notFound.set(true);
              } else {
                this.error.set(safeError.message);
              }
              this.requestId.set(safeError.requestId ?? null);
              return of(null);
            }),
          );
        }),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((invoice) => {
        if (invoice) {
          this.invoice.set(invoice);
        }
      });
  }

  retry(): void {
    this.error.set(null);
    this.actionError.set(null);
    this.retrySubject.next();
  }

  print(): void {
    if (!this.canPrint()) {
      return;
    }

    const inv = this.invoice();
    if (!inv) {
      return;
    }

    this.printing.set(true);
    this.actionError.set(null);
    this.requestId.set(null);

    this.invoiceApi
      .print(inv.id)
      .pipe(
        finalize(() => this.printing.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (updatedInvoice) => {
          this.invoice.set(updatedInvoice);
          this.snackBar.open('Fatura finalizada com sucesso!', 'Fechar', { duration: 3000 });
          this.actionError.set(null);
          this.requestId.set(null);
          window.print();
        },
        error: (err: HttpErrorResponse) => {
          this.handlePrintError(err);
        },
      });
  }

  private handlePrintError(error: HttpErrorResponse): void {
    const safeError = extractSafeHttpApiError(error);
    this.actionError.set(safeError);
    this.requestId.set(safeError.requestId ?? null);

    if (safeError.code === 'INVOICE_NOT_OPEN') {
      this.loadInvoice();
    }
  }

  private loadInvoice(): void {
    const id = this.invoice()?.id;
    if (!id) {
      this.actionError.set(null);
      return;
    }
    this.loading.set(true);
    this.invoiceApi
      .get(id)
      .pipe(
        finalize(() => this.loading.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (invoice) => this.invoice.set(invoice),
        error: (err: HttpErrorResponse) => {
          const safeError = extractSafeHttpApiError(err);
          this.actionError.set(null);
          if (safeError.code === 'INVOICE_NOT_FOUND' || err.status === 404) {
            this.notFound.set(true);
          } else {
            this.error.set(safeError.message);
          }
          this.requestId.set(safeError.requestId ?? null);
        },
      });
  }

  translateStatus(status: 'OPEN' | 'CLOSED'): string {
    return status === 'OPEN' ? 'Aberta' : 'Fechada';
  }

  protected async copyRequestId(requestId: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(requestId);
      this.snackBar.open('ID da solicitação copiado.', 'Fechar', { duration: 2500 });
    } catch {
      this.snackBar.open(`Não foi possível copiar. ID: ${requestId}`, 'Fechar', { duration: 5000 });
    }
  }
}
