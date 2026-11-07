/** A terminal mutation must never be replayed merely because Enter is pressed.
 * Only a local refusal before dispatch can safely return to the same review. */
export function canReviewOperationFailure(input: {
  dispatched: boolean;
  succeeded: boolean;
  cancelled: boolean;
  reconciled: boolean;
}): boolean {
  return (
    !input.dispatched &&
    !input.succeeded &&
    !input.cancelled &&
    input.reconciled
  );
}
