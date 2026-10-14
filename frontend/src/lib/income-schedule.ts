import { monthlyAmount } from '@/data/labels';
import { currentMonthKey, dayKeyOf, todayKey } from '@/lib/dates';
import { isWithinSchedule, scheduledDayThisMonth } from '@/lib/schedule';
import type { Income } from '@/types/finance';

export { buildScheduleStartsAt } from '@/lib/schedule';

/** Month (São Paulo calendar) used for receivedForMonth / receiptHoldForMonth. */
export function incomeMonthKey(now: Date = new Date()) {
  return currentMonthKey(now);
}

export function isSalaryWaitingForConfirmation(
  income: Income,
  now: Date = new Date(),
) {
  return (
    income.type === 'salario' &&
    income.frequency === 'mensal' &&
    income.receiptHoldForMonth === incomeMonthKey(now)
  );
}

export function isSalaryManuallyReceived(
  income: Income,
  now: Date = new Date(),
) {
  return (
    income.type === 'salario' &&
    income.frequency === 'mensal' &&
    income.receivedForMonth === incomeMonthKey(now)
  );
}

/** Avulsas só afetam o saldo quando um recebimento real foi registrado. */
function oneOffReceivedAmountThisMonth(income: Income, now: Date) {
  const today = todayKey(now);
  const month = incomeMonthKey(now);
  return (income.receipts ?? []).reduce((sum, receipt) => {
    const receivedOn = dayKeyOf(receipt.receivedAt);
    if (!receivedOn || receivedOn > today || receivedOn.slice(0, 7) !== month) {
      return sum;
    }
    const amount = Number(receipt.amount);
    return sum + (Number.isFinite(amount) && amount > 0 ? amount : 0);
  }, 0);
}

/** Crédito automático pela agenda é permitido apenas a entradas recorrentes. */
export function isIncomeAutoReceivedThisMonth(
  income: Income,
  now: Date = new Date(),
) {
  if (income.frequency === 'unica') return false;
  const today = todayKey(now);
  const receiveDay = scheduledDayThisMonth(income.receiveDay ?? 1, now);
  if (today < receiveDay) return false;
  return isWithinSchedule(receiveDay, income);
}

/** Entrada já entrou no saldo do mês corrente, incluindo confirmação manual do salário. */
export function isIncomeReceivedThisMonth(
  income: Income,
  now: Date = new Date(),
) {
  if (income.frequency === 'unica') {
    return oneOffReceivedAmountThisMonth(income, now) > 0;
  }
  if (isSalaryManuallyReceived(income, now)) return true;
  if (isSalaryWaitingForConfirmation(income, now)) return false;
  return isIncomeAutoReceivedThisMonth(income, now);
}

export function incomeContributionThisMonth(
  income: Income,
  now: Date = new Date(),
) {
  if (income.frequency === 'unica') {
    return oneOffReceivedAmountThisMonth(income, now);
  }
  if (!isIncomeReceivedThisMonth(income, now)) return 0;
  return monthlyAmount(income.amount, income.frequency);
}
