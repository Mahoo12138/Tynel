function inspect(value: number | undefined): number {
  if (value !== undefined) {
    return value;
  }
  return 0;
}
