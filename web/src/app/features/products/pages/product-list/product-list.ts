import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { MatTableModule } from '@angular/material/table';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatCardModule } from '@angular/material/card';
import { Product } from '../../../../core/models/product.model';
import { ProductApiService } from '../../services/product-api.service';
import { catchError, finalize, of } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { extractSafeHttpApiError, SafeApiError } from '../../../../core/models/api-error.model';

@Component({
  selector: 'app-product-list',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    MatTableModule,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    MatCardModule,
  ],
  styleUrl: './product-list.scss',
  templateUrl: './product-list.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProductList {
  private readonly destroyRef = inject(DestroyRef);
  private readonly productApiService = inject(ProductApiService);

  protected readonly loading = signal(false);
  protected readonly products = signal<Product[]>([]);
  protected readonly error = signal<SafeApiError | null>(null);

  protected readonly displayedColumns: string[] = ['code', 'description', 'balance'];

  constructor() {
    this.loadProducts();
  }

  protected loadProducts(): void {
    this.loading.set(true);
    this.error.set(null);

    this.productApiService
      .list()
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loading.set(false)),
        catchError((error: unknown) => {
          const safeError = extractSafeHttpApiError(error);
          this.error.set(safeError);
          return of([] as Product[]);
        }),
      )
      .subscribe((data) => {
        this.products.set(data);
      });
  }

  protected retryLoad(): void {
    this.loadProducts();
  }
}
