import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import {
  AbstractControl,
  FormArray,
  FormBuilder,
  FormControl,
  FormGroup,
  ReactiveFormsModule,
  ValidationErrors,
  Validators,
} from '@angular/forms';
import { Router, RouterModule } from '@angular/router';
import { MatSnackBar } from '@angular/material/snack-bar';
import { InvoiceApiService } from '../../services/invoice-api.service';
import { ProductApiService } from '../../../products/services/product-api.service';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs/operators';
import { Product } from '../../../../core/models/product.model';
import { HttpErrorResponse } from '@angular/common/http';
import { extractSafeHttpApiError, SafeApiError } from '../../../../core/models/api-error.model';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatSelectModule } from '@angular/material/select';
import { MatInputModule } from '@angular/material/input';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatCardModule } from '@angular/material/card';

function uniqueProductsValidator(control: AbstractControl): ValidationErrors | null {
  const formArray = control as FormArray;
  const productIds = formArray.controls
    .map((c) => c.get('productId')?.value)
    .filter((val) => !!val);
  const duplicates = productIds.some((val, index) => productIds.indexOf(val) !== index);
  return duplicates ? { duplicateProducts: true } : null;
}

type InvoiceItemForm = FormGroup<{
  productId: FormControl<string | null>;
  quantity: FormControl<number | null>;
}>;

@Component({
  selector: 'app-invoice-create',
  standalone: true,
  imports: [
    ReactiveFormsModule,
    RouterModule,
    MatButtonModule,
    MatIconModule,
    MatSelectModule,
    MatInputModule,
    MatFormFieldModule,
    MatProgressSpinnerModule,
    MatCardModule,
  ],
  templateUrl: './invoice-create.html',
  styleUrl: './invoice-create.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class InvoiceCreate implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly router = inject(Router);
  private readonly snackBar = inject(MatSnackBar);
  private readonly invoiceApi = inject(InvoiceApiService);
  private readonly productApi = inject(ProductApiService);
  private readonly destroyRef = inject(DestroyRef);

  readonly products = signal<Product[]>([]);
  readonly isLoadingProducts = signal(true);
  readonly isSaving = signal(false);
  readonly productLoadError = signal<SafeApiError | null>(null);
  readonly errorMessage = signal<SafeApiError | null>(null);

  readonly form = this.fb.group({
    items: this.fb.array<InvoiceItemForm>([], [uniqueProductsValidator]),
  });

  get items(): FormArray<InvoiceItemForm> {
    return this.form.get('items') as FormArray<InvoiceItemForm>;
  }

  ngOnInit(): void {
    this.addItem();
    this.loadProducts();
  }

  loadProducts(): void {
    this.isLoadingProducts.set(true);
    this.productLoadError.set(null);
    this.productApi
      .list()
      .pipe(
        finalize(() => this.isLoadingProducts.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (products) => this.products.set(products),
        error: (error: HttpErrorResponse) => {
          this.productLoadError.set(extractSafeHttpApiError(error));
        },
      });
  }

  createItem(): InvoiceItemForm {
    return this.fb.group({
      productId: this.fb.control<string | null>(null, [Validators.required]),
      quantity: this.fb.control<number | null>(null, [
        Validators.required,
        Validators.min(1),
        Validators.pattern('^[0-9]*$'),
      ]),
    });
  }

  addItem(): void {
    this.items.push(this.createItem());
  }

  removeItem(index: number): void {
    if (this.items.length > 1) {
      this.items.removeAt(index);
    }
  }

  onSubmit(): void {
    if (this.form.invalid || this.isSaving()) {
      this.form.markAllAsTouched();
      return;
    }

    this.isSaving.set(true);
    this.form.disable();
    this.errorMessage.set(null);

    const formValue = this.form.getRawValue();
    const items = formValue.items.map((item) => ({
      product_id: Number(item.productId!),
      quantity: Number(item.quantity!),
    }));

    this.invoiceApi
      .create({ items })
      .pipe(
        finalize(() => {
          this.isSaving.set(false);
          this.form.enable();
        }),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (invoice) => {
          this.snackBar.open(`Invoice #${invoice.number} created successfully`, 'Close', {
            duration: 3000,
          });
          this.router.navigate(['/invoices', invoice.id]);
        },
        error: (error: HttpErrorResponse) => {
          this.handleSaveError(error);
        },
      });
  }

  private handleSaveError(error: HttpErrorResponse): void {
    this.errorMessage.set(extractSafeHttpApiError(error));
  }
}
