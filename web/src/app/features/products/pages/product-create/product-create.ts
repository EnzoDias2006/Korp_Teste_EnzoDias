import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  AbstractControl,
  FormBuilder,
  FormControl,
  ReactiveFormsModule,
  ValidationErrors,
  Validators,
} from '@angular/forms';
import { Router, RouterModule } from '@angular/router';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatIconModule } from '@angular/material/icon';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpErrorResponse } from '@angular/common/http';
import { EMPTY, catchError, finalize } from 'rxjs';
import { ProductApiService } from '../../services/product-api.service';
import { CreateProductRequest } from '../../../../core/models/product.model';
import { extractSafeHttpApiError, SafeApiError } from '../../../../core/models/api-error.model';

function integerValidator(control: AbstractControl): ValidationErrors | null {
  if (control.value === null || control.value === undefined || control.value === '') {
    return { required: true };
  }

  const value = Number(control.value);
  return Number.isInteger(value) ? null : { notInteger: true };
}

@Component({
  selector: 'app-product-create',
  standalone: true,
  imports: [
    CommonModule,
    ReactiveFormsModule,
    RouterModule,
    MatCardModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatProgressSpinnerModule,
    MatIconModule,
  ],
  templateUrl: './product-create.html',
  styleUrl: './product-create.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProductCreate {
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);
  private readonly router = inject(Router);
  private readonly productApi = inject(ProductApiService);

  readonly saving = signal(false);
  readonly submitInFlight = signal(false);
  readonly errorMessage = signal<SafeApiError | null>(null);

  readonly form = this.fb.group({
    code: this.fb.nonNullable.control('', [Validators.required, Validators.pattern(/\S/)]),
    description: this.fb.nonNullable.control('', [Validators.required, Validators.pattern(/\S/)]),
    balance: this.fb.control<number | null>(0, [
      Validators.required,
      Validators.min(0),
      integerValidator,
    ]),
  });

  get code(): FormControl<string> {
    return this.form.controls.code;
  }

  get description(): FormControl<string> {
    return this.form.controls.description;
  }

  get balance(): FormControl<number | null> {
    return this.form.controls.balance;
  }

  onSubmit(): void {
    if (this.form.invalid) {
      this.markInvalidControlsTouched();
      return;
    }

    if (this.saving() || this.submitInFlight()) {
      return;
    }

    this.submitInFlight.set(true);
    this.saving.set(true);

    const request: CreateProductRequest = {
      code: this.code.value.trim(),
      description: this.description.value.trim(),
      balance: Number(this.balance.value),
    };

    this.productApi
      .create(request)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => {
          this.saving.set(false);
          this.submitInFlight.set(false);
        }),
        catchError((error: HttpErrorResponse) => {
          this.handleError(error);
          return EMPTY;
        }),
      )
      .subscribe(() => {
        this.errorMessage.set(null);
        void this.router.navigate(['/products']);
      });
  }

  private markInvalidControlsTouched(): void {
    Object.values(this.form.controls).forEach((control: AbstractControl) => {
      if (control.invalid) {
        control.markAsTouched();
      }
    });
  }

  private handleError(error: HttpErrorResponse): void {
    this.errorMessage.set(extractSafeHttpApiError(error));
  }
}
